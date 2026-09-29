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

func TestBranchSwitchPreflightReportsFixedFailureAndKeepsCurrent(t *testing.T) {
	ts := fileRestoreGame(t)
	path := filepath.Join(ts.saveDir, "slot.txt")
	if err := os.WriteFile(path, []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	branch, err := ts.daemon.Snapshots.CreateBranch("game", "incoming", true)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := ts.daemon.Store.ListSnapshots("game", branch)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("incoming snapshot: %v, %d", err, len(snaps))
	}
	if err := os.WriteFile(snaps[0].ZipPath, []byte("synthetic broken archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, data := ts.do(t, http.MethodPost, "/api/games/game/branch/switch", map[string]string{"name": branch})
	var code string
	if err := json.Unmarshal(data["code"], &code); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict || code != "restore_archive" || strings.Contains(fmt.Sprint(data), snaps[0].ZipPath) {
		t.Fatalf("branch switch exposed an unsafe failure: status %d, code %s", resp.StatusCode, code)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "current" {
		t.Fatal("current save changed")
	}
	if game, err := ts.daemon.Store.GetGame("game"); err != nil || game.ActiveBranch != "main" {
		t.Fatal("active branch changed")
	}
}
