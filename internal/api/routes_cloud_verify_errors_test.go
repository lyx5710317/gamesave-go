package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/cloud"
)

func TestCloudVerificationErrorCodesDoNotExposePrivateDetails(t *testing.T) {
	for _, tc := range []struct {
		code string
		err  error
	}{
		{"disabled", cloud.ErrCloudDisabled},
		{"configuration", errors.New("application password is not configured")},
		{"authentication", cloud.ErrJianguoyunAuth},
		{"permission", cloud.ErrJianguoyunPermission},
		{"quota", cloud.ErrJianguoyunQuota},
		{"rate_limit", cloud.ErrJianguoyunRateLimit},
		{"network", cloud.ErrJianguoyunNetwork},
		{"missing", cloud.ErrJianguoyunMissing},
		{"incomplete_inventory", cloud.ErrJianguoyunIncomplete},
		{"ambiguous", cloud.ErrRemoteSnapshotAmbiguous},
		{"unsafe_archive", cloud.ErrUnsafeSnapshotArchive},
		{"size_mismatch", cloud.ErrSnapshotSizeMismatch},
		{"integrity", cloud.ErrSnapshotArchiveIntegrity},
		{"integrity", cloud.ErrJianguoyunIntegrity},
		{"local_io", &os.PathError{Op: "open", Path: "synthetic-private-path", Err: os.ErrPermission}},
		{"failed", errors.New("unexpected remote response")},
	} {
		t.Run(tc.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			err := fmt.Errorf("synthetic-private-detail user@example.invalid Authorization: synthetic: %w", tc.err)
			writeCloudVerificationError(recorder, http.StatusBadGateway, err)
			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusBadGateway || body["code"] != "cloud_verify_"+tc.code {
				t.Fatalf("incorrect failure response: %d, %v", recorder.Code, body)
			}
			if len(body) != 2 || body["error"] != "cloud snapshot could not be verified; no saves were changed" {
				t.Fatalf("non-fixed response: %v", body)
			}
			for _, forbidden := range []string{"synthetic", "example.invalid", "Authorization", "private-path"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("private diagnostic leaked: %s", forbidden)
				}
			}
		})
	}
}

type verifyFailureProvider struct {
	files         []cloud.CloudFile
	listErr       error
	downloadErr   error
	data          []byte
	downloadPaths []string
}

func (p *verifyFailureProvider) Upload(string, string) error      { return errors.New("unused") }
func (p *verifyFailureProvider) Delete(cloud.CloudFile) error     { return errors.New("unused") }
func (p *verifyFailureProvider) List() ([]cloud.CloudFile, error) { return p.files, p.listErr }
func (p *verifyFailureProvider) Download(_ string, destination string) error {
	p.downloadPaths = append(p.downloadPaths, destination)
	if err := os.WriteFile(destination, p.data, 0o600); err != nil {
		return err
	}
	return p.downloadErr
}

func TestCloudVerifyFailureCategoriesKeepLocalArchiveAndCleanTemporaryFiles(t *testing.T) {
	data := verifyTestZIP(t, "synthetic remote progress")
	const name = "game__main__snap.zip"
	file := cloud.CloudFile{Name: name, SizeBytes: int64(len(data))}
	for _, tc := range []struct {
		name, code string
		provider   verifyFailureProvider
		status     int
	}{
		{"list auth", "authentication", verifyFailureProvider{listErr: cloud.ErrJianguoyunAuth}, 502},
		{"download network", "network", verifyFailureProvider{files: []cloud.CloudFile{file}, data: data[:10], downloadErr: cloud.ErrJianguoyunNetwork}, 502},
		{"size mismatch", "size_mismatch", verifyFailureProvider{files: []cloud.CloudFile{file}, data: data[:len(data)-1]}, 502},
		{"invalid ZIP", "integrity", verifyFailureProvider{files: []cloud.CloudFile{{Name: name, SizeBytes: 7}}, data: []byte("invalid")}, 502},
		{"missing", "missing", verifyFailureProvider{}, 404},
		{"duplicate", "ambiguous", verifyFailureProvider{files: []cloud.CloudFile{file, file}}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := startTestServer(t)
			provider := &tc.provider
			if err := ts.daemon.Cloud.RegisterProvider("verify_failure", provider); err != nil {
				t.Fatal(err)
			}
			cfg, err := ts.daemon.Store.GetCloudConfig()
			if err != nil {
				t.Fatal(err)
			}
			cfg.Enabled, cfg.Provider = true, "verify_failure"
			if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
				t.Fatal(err)
			}
			settings, err := ts.daemon.Store.GetSettings()
			if err != nil {
				t.Fatal(err)
			}
			localPath := filepath.Join(settings.BackupsDir, "game", "main", "snap.zip")
			if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
				t.Fatal(err)
			}
			localData := verifyTestZIP(t, "synthetic local progress")
			if err := os.WriteFile(localPath, localData, 0o600); err != nil {
				t.Fatal(err)
			}
			response, body := ts.do(t, http.MethodPost, "/api/cloud/verify/game", map[string]any{"fileName": name})
			if response.StatusCode != tc.status || string(body["code"]) != `"cloud_verify_`+tc.code+`"` {
				t.Fatalf("failure response = %d %v", response.StatusCode, body)
			}
			got, err := os.ReadFile(localPath)
			if err != nil || string(got) != string(localData) {
				t.Fatalf("local archive changed: %v", err)
			}
			for _, tempPath := range provider.downloadPaths {
				if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
					t.Fatalf("temporary file not removed: %v", err)
				}
			}
		})
	}
}
