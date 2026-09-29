package cliapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCloudPushReturnsFailureWhenAnyUploadFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	var payload atomic.Value
	payload.Store(`{"uploaded":1,"skipped":2,"failed":1}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/cloud/sync-local/game" {
			t.Errorf("unexpected cloud request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, payload.Load())
	}))
	defer server.Close()
	dataDir := filepath.Join(home, ".opensave")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	addr := strings.TrimPrefix(server.URL, "http://")
	if err := os.WriteFile(filepath.Join(dataDir, "daemon.addr"), []byte(addr), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cloudPush(false, []string{"game"}); code != 1 {
		t.Fatalf("cloudPush exit code = %d, want 1 for partial failure", code)
	}
	if code := cloudPush(true, []string{"game"}); code != 1 {
		t.Fatalf("cloudPush --json exit code = %d, want 1 for partial failure", code)
	}
	payload.Store(`{"uploaded":1,"skipped":0,"conflicts":2,"failed":0}`)
	if code := cloudPush(false, []string{"game"}); code != 1 {
		t.Fatalf("cloudPush exit code = %d, want 1 for unverified remote collisions", code)
	}
	if code := cloudPush(true, []string{"game"}); code != 1 {
		t.Fatalf("cloudPush --json exit code = %d, want 1 for unverified remote collisions", code)
	}
}
