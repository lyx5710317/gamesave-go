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

	"github.com/opensave/opensave/internal/snapshot"
)

func TestRestorePreflightCategoriesHidePrivateErrors(t *testing.T) {
	for _, tc := range []struct {
		code string
		err  error
	}{
		{"restore_archive", snapshot.ErrRestoreArchive},
		{"restore_location", snapshot.ErrRestoreLocation},
		{"restore_safety", snapshot.ErrRestoreSafety},
		{"restore_changed", snapshot.ErrRestoreChanged},
	} {
		r := httptest.NewRecorder()
		if !writeRestorePreflightError(r, fmt.Errorf("synthetic-private-path: %w", tc.err)) {
			t.Fatal("missing category")
		}
		var body map[string]string
		if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if r.Code != http.StatusConflict || body["code"] != tc.code || len(body) != 2 || strings.Contains(r.Body.String(), "synthetic-private") {
			t.Fatal("non-fixed failure")
		}
	}
	r := httptest.NewRecorder()
	if writeRestorePreflightError(r, errors.New("unknown")) || r.Body.Len() != 0 {
		t.Fatal("unknown error mapped")
	}
}

func TestRollbackPreflightPreservesCurrentSave(t *testing.T) {
	ts := fileRestoreGame(t)
	path := filepath.Join(ts.saveDir, "slot.txt")
	if err := os.WriteFile(path, []byte("snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := ts.daemon.Snapshots.Create("game", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snap.ZipPath, []byte("synthetic broken archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, data := ts.do(t, http.MethodPost, "/api/games/game/rollback", map[string]string{"snapshotId": snap.ID})
	var code string
	if err := json.Unmarshal(data["code"], &code); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict || code != "restore_archive" {
		t.Fatalf("wrong rollback failure: %d", resp.StatusCode)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "current" {
		t.Fatal("current state changed")
	}
}
