package api

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/store"
)

func TestCloudRestoreRejectsConflictingArchiveBeforeImportOrSaveChanges(t *testing.T) {
	cases := [][]string{
		{"private-slot.sav", "private-slot.sav"},
		{"folder", "folder/private-slot.sav"},
		{"folder/../private-slot.sav"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, []string{"Private-slot.sav", "private-slot.sav"}, []string{"NUL.sav"})
	}
	for _, names := range cases {
		t.Run(names[0], func(t *testing.T) {
			ts := startTestServer(t)
			localSave := filepath.Join(ts.saveDir, "slot.sav")
			original := []byte("synthetic local progress")
			if err := os.WriteFile(localSave, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ts.daemon.Store.CreateGame(store.Game{ID: "game", Name: "Test", SavePath: ts.saveDir}); err != nil {
				t.Fatal(err)
			}
			remoteDir := t.TempDir()
			cfg, err := ts.daemon.Store.GetCloudConfig()
			if err != nil {
				t.Fatal(err)
			}
			cfg.Enabled, cfg.Provider, cfg.URL = true, "local", remoteDir
			if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
				t.Fatal(err)
			}
			var archive bytes.Buffer
			w := zip.NewWriter(&archive)
			for _, name := range names {
				entry, err := w.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write([]byte("synthetic remote data")); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			const fileName = "game__main__snap.zip"
			remotePath := filepath.Join(remoteDir, fileName)
			if err := os.WriteFile(remotePath, archive.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			request := map[string]any{"fileName": fileName}
			resp, verifyBody := ts.do(t, http.MethodPost, "/api/cloud/verify/game", request)
			if resp.StatusCode != http.StatusBadGateway || string(verifyBody["code"]) != `"cloud_verify_unsafe_archive"` {
				t.Fatalf("unsafe verification status = %d", resp.StatusCode)
			}
			resp, body := ts.do(t, http.MethodPost, "/api/cloud/restore/game", request)
			if resp.StatusCode != http.StatusBadGateway || !strings.Contains(string(body["error"]), cloud.ErrUnsafeSnapshotArchive.Error()) {
				t.Fatalf("unsafe restore response = %d, %v", resp.StatusCode, body)
			}
			if strings.Contains(string(body["error"]), "private-slot") {
				t.Fatal("untrusted entry name leaked in restore response")
			}
			if got, err := os.ReadFile(localSave); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("local save changed: %v", err)
			}
			if got, err := os.ReadFile(remotePath); err != nil || !bytes.Equal(got, archive.Bytes()) {
				t.Fatalf("remote archive changed: %v", err)
			}
			snaps, err := ts.daemon.Store.ListSnapshots("game", "main")
			if err != nil || len(snaps) != 0 {
				t.Fatalf("unsafe snapshot imported or restore attempted: %v, %v", snaps, err)
			}
			settings, err := ts.daemon.Store.GetSettings()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(settings.BackupsDir, "game", "main", "snap.zip")); !os.IsNotExist(err) {
				t.Fatalf("unsafe archive published: %v", err)
			}
			parts, err := filepath.Glob(filepath.Join(settings.BackupsDir, "game", "main", ".opensave-cloud-*.part"))
			if err != nil || len(parts) != 0 {
				t.Fatalf("temporary downloads remain: %v, %v", parts, err)
			}
		})
	}
}
