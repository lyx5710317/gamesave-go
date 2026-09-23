package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/vaultmeta"
)

func TestCloudJoinLocalPreviewIsReadOnlyAndPathFree(t *testing.T) {
	ts := startTestServer(t)
	ts.daemon.Scanner.SteamUserdataPaths = []string{}
	ts.daemon.Scanner.ResolveAppName = nil
	if err := os.WriteFile(filepath.Join(ts.saveDir, "slot.sav"), []byte("tracked save"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ts.daemon.Store.CreateGame(store.Game{ID: "tracked", Name: "Tracked Game", SavePath: ts.saveDir}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	candidatePath := filepath.Join(root, "PreviewCandidate")
	if err := os.MkdirAll(candidatePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidatePath, "progress.dat"), []byte("candidate save"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := ts.daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.CustomScanPaths = []string{root}
	if err := ts.daemon.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	resp, body := ts.do(t, http.MethodGet, "/api/cloud/join/local-preview", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("local preview status = %d, body = %v", resp.StatusCode, body)
	}
	var scan vaultmeta.LocalScan
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &scan); err != nil {
		t.Fatal(err)
	}
	if len(scan.Library.Games) != 1 || scan.Library.Games[0].GameID != "tracked" ||
		scan.Library.Games[0].FileCount != 1 || len(scan.Library.Games[0].ContentHash) != 64 {
		t.Fatalf("tracked library = %#v", scan.Library)
	}
	foundCandidate := false
	for _, candidate := range scan.Candidates {
		if candidate.Name == "PreviewCandidate" {
			foundCandidate = candidate.Measured && candidate.FileCount == 1
		}
	}
	if !foundCandidate {
		t.Fatalf("measured candidate missing: %#v", scan.Candidates)
	}
	if strings.Contains(string(raw), ts.saveDir) || strings.Contains(string(raw), root) ||
		strings.Contains(string(raw), "savePath") {
		t.Fatalf("local preview leaked a save path: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(ts.saveDir, "slot.sav")); err != nil {
		t.Fatalf("preview changed tracked save: %v", err)
	}
}

func TestCloudJoinLocalPreviewFailsClosedForMissingTrackedSave(t *testing.T) {
	ts := startTestServer(t)
	missing := filepath.Join(t.TempDir(), "missing")
	if err := ts.daemon.Store.CreateGame(store.Game{ID: "missing", Name: "Missing", SavePath: missing}); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodGet, "/api/cloud/join/local-preview", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("missing tracked save status = %d, body = %v", resp.StatusCode, body)
	}
	if _, ok := body["library"]; ok {
		t.Fatalf("failed preview returned partial library: %v", body)
	}
}
