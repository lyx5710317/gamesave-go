package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/store"
)

type failingUploadProvider struct{}

func (failingUploadProvider) Upload(_, _ string) error         { return errors.New("access_token=do-not-show") }
func (failingUploadProvider) List() ([]cloud.CloudFile, error) { return []cloud.CloudFile{}, nil }
func (failingUploadProvider) Download(_, _ string) error       { return nil }
func (failingUploadProvider) Delete(cloud.CloudFile) error     { return nil }

type existingUploadProvider struct {
	files   []cloud.CloudFile
	uploads int
}

type racingUploadProvider struct {
	lists   int
	uploads int
}

func (p *racingUploadProvider) Upload(_, _ string) error { p.uploads++; return nil }
func (p *racingUploadProvider) List() ([]cloud.CloudFile, error) {
	p.lists++
	if p.lists == 1 {
		return nil, nil
	}
	return []cloud.CloudFile{{Name: "game__main__snap.zip"}}, nil
}
func (p *racingUploadProvider) Download(_, _ string) error   { return nil }
func (p *racingUploadProvider) Delete(cloud.CloudFile) error { return nil }

func (p *existingUploadProvider) Upload(_, _ string) error {
	p.uploads++
	return nil
}
func (p *existingUploadProvider) List() ([]cloud.CloudFile, error) { return p.files, nil }
func (p *existingUploadProvider) Download(_, _ string) error       { return nil }
func (p *existingUploadProvider) Delete(cloud.CloudFile) error     { return nil }

func TestCloudUploadsEndpointShowsSanitizedCurrentRunActivity(t *testing.T) {
	ts := startTestServer(t)
	resp, body := ts.do(t, http.MethodGet, "/api/cloud/uploads", nil)
	if resp.StatusCode != http.StatusOK || string(body["uploads"]) != "[]" {
		t.Fatalf("empty upload activity = %d, %v", resp.StatusCode, body)
	}
	destination := t.TempDir()
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	cfg.Provider = "local"
	cfg.URL = destination
	if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "private-save.zip")
	if err := os.WriteFile(source, []byte("test archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ts.daemon.Cloud.Upload(source, "game__main__snap.zip"); err != nil {
		t.Fatal(err)
	}
	resp, body = ts.do(t, http.MethodGet, "/api/cloud/uploads", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activity status = %d, body = %v", resp.StatusCode, body)
	}
	var records []struct {
		GameID     string `json:"gameId"`
		SnapshotID string `json:"snapshotId"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(body["uploads"], &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].GameID != "game" || records[0].SnapshotID != "snap" || records[0].Status != "succeeded" {
		t.Fatalf("activity = %#v", records)
	}
	if strings.Contains(string(body["uploads"]), source) || strings.Contains(string(body["uploads"]), destination) {
		t.Fatalf("activity leaked filesystem path: %s", body["uploads"])
	}
}

func TestCloudSyncLocalReportsTransferFailures(t *testing.T) {
	ts := startTestServer(t)
	if err := ts.daemon.Cloud.RegisterProvider("failing_test", failingUploadProvider{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider = true, "failing_test"
	if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := ts.daemon.Store.CreateGame(store.Game{ID: "game", Name: "Game", SavePath: ts.saveDir}); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "save.zip")
	if err := os.WriteFile(zipPath, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{
		ID: "snap", GameID: "game", BranchName: "main", Timestamp: "2026-09-23T00:00:00Z",
		ZipPath: zipPath, SizeBytes: int64(len("archive")),
	}); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/cloud/sync-local/game", nil)
	if resp.StatusCode != http.StatusOK || string(body["uploaded"]) != "0" ||
		string(body["skipped"]) != "0" || string(body["failed"]) != "1" {
		t.Fatalf("failed upload reported as success or skip: %d, %v", resp.StatusCode, body)
	}
	resp, body = ts.do(t, http.MethodGet, "/api/cloud/uploads", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body["uploads"]), `"failure":"transfer"`) ||
		strings.Contains(string(body["uploads"]), "do-not-show") {
		t.Fatalf("failure activity leaked secret or lost state: %d, %v", resp.StatusCode, body)
	}
}

func TestCloudSyncLocalBlocksListedRemoteName(t *testing.T) {
	for _, size := range []int64{7, 3, 0} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			ts := startTestServer(t)
			provider := &existingUploadProvider{files: []cloud.CloudFile{{Name: "game__main__snap.zip", SizeBytes: size}}}
			if err := ts.daemon.Cloud.RegisterProvider("existing_test", provider); err != nil {
				t.Fatal(err)
			}
			cfg, err := ts.daemon.Store.GetCloudConfig()
			if err != nil {
				t.Fatal(err)
			}
			cfg.Enabled, cfg.Provider = true, "existing_test"
			if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
				t.Fatal(err)
			}
			if err := ts.daemon.Store.CreateGame(store.Game{ID: "game", Name: "Game", SavePath: ts.saveDir}); err != nil {
				t.Fatal(err)
			}
			zipPath := filepath.Join(t.TempDir(), "save.zip")
			if err := os.WriteFile(zipPath, []byte("archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{
				ID: "snap", GameID: "game", BranchName: "main", Timestamp: "2026-09-23T00:00:00Z",
				ZipPath: zipPath, SizeBytes: 7,
			}); err != nil {
				t.Fatal(err)
			}
			resp, body := ts.do(t, http.MethodPost, "/api/cloud/sync-local/game", nil)
			if resp.StatusCode != http.StatusOK || string(body["uploaded"]) != "0" ||
				string(body["skipped"]) != "0" || string(body["conflicts"]) != "1" ||
				provider.uploads != 0 {
				t.Fatalf("unverified remote was overwritten or marked current: %d, %v, uploads=%d", resp.StatusCode, body, provider.uploads)
			}
		})
	}
}

func TestCloudSyncLocalRechecksNameBeforeUpload(t *testing.T) {
	ts := startTestServer(t)
	provider := &racingUploadProvider{}
	if err := ts.daemon.Cloud.RegisterProvider("racing_test", provider); err != nil {
		t.Fatal(err)
	}
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider = true, "racing_test"
	if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := ts.daemon.Store.CreateGame(store.Game{ID: "game", Name: "Game", SavePath: ts.saveDir}); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "save.zip")
	if err := os.WriteFile(zipPath, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{
		ID: "snap", GameID: "game", BranchName: "main", Timestamp: "2026-09-23T00:00:00Z",
		ZipPath: zipPath, SizeBytes: int64(len("archive")),
	}); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/cloud/sync-local/game", nil)
	if resp.StatusCode != http.StatusOK || string(body["conflicts"]) != "1" ||
		string(body["uploaded"]) != "0" || provider.lists != 2 || provider.uploads != 0 {
		t.Fatalf("racing remote snapshot was overwritten: status=%d body=%v lists=%d uploads=%d", resp.StatusCode, body, provider.lists, provider.uploads)
	}
}
