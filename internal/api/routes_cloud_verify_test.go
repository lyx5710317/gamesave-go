package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func verifyTestZIP(t *testing.T, contents string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	f, err := w.Create("slot.sav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestCloudVerifyChecksRemoteWithoutRestoringOrChangingLocal(t *testing.T) {
	ts := startTestServer(t)
	remoteDir := t.TempDir()
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider, cfg.URL = true, "local", remoteDir
	if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	const name = "game__main__snap.zip"
	remote := verifyTestZIP(t, "remote progress")
	if err := os.WriteFile(filepath.Join(remoteDir, name), remote, 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := ts.daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(settings.BackupsDir, "game", "main", "snap.zip")
	request := map[string]any{"fileName": name}
	check := func(want string) {
		t.Helper()
		resp, body := ts.do(t, http.MethodPost, "/api/cloud/verify/game", request)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("verify status = %d, body = %v", resp.StatusCode, body)
		}
		var result struct {
			LocalComparison string `json:"localComparison"`
			SizeBytes       int64  `json:"sizeBytes"`
		}
		raw, _ := json.Marshal(body)
		if err := json.Unmarshal(raw, &result); err != nil || result.LocalComparison != want || result.SizeBytes != int64(len(remote)) {
			t.Fatalf("verify result = %#v, %v", result, err)
		}
	}
	check("unavailable")
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("verify created local archive: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, remote, 0o600); err != nil {
		t.Fatal(err)
	}
	check("identical")
	local := verifyTestZIP(t, "local progress")
	if err := os.WriteFile(localPath, local, 0o600); err != nil {
		t.Fatal(err)
	}
	check("different")
	if got, err := os.ReadFile(localPath); err != nil || !bytes.Equal(got, local) {
		t.Fatalf("verify changed local archive: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(remoteDir, name)); err != nil || !bytes.Equal(got, remote) {
		t.Fatalf("verify changed remote archive: %v", err)
	}
	resp, _ := ts.do(t, http.MethodPost, "/api/cloud/verify/game", map[string]any{"fileName": "other__main__snap.zip"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("other game's archive accepted: %d", resp.StatusCode)
	}
	resp, _ = ts.do(t, http.MethodPost, "/api/cloud/verify/game", map[string]any{"fileName": "game__..__snap.zip"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsafe archive name accepted: %d", resp.StatusCode)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, name), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, _ = ts.do(t, http.MethodPost, "/api/cloud/verify/game", request)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("corrupt remote accepted: %d", resp.StatusCode)
	}
	if got, err := os.ReadFile(localPath); err != nil || !bytes.Equal(got, local) {
		t.Fatalf("failed verify changed local archive: %v", err)
	}
}
