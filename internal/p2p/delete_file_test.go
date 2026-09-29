package p2p

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

func TestWANDeleteUsesNamedSaveRootAndReportsFailure(t *testing.T) {
	e, s := newMatchTestEngine(t)
	primary := t.TempDir()
	extra := t.TempDir()
	game := store.Game{ID: "test-game", Name: "Test game", SavePath: primary}
	if err := s.CreateGame(game); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot(game.ID, "config", extra); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{primary, extra} {
		if err := os.WriteFile(filepath.Join(dir, "slot.sav"), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w := &WanClient{engine: e}
	status, _ := w.serveDeleteFile("/delete-file/"+game.ID, json.RawMessage(`{"root":"config","relPath":"slot.sav"}`), "")
	if status != 200 {
		t.Fatalf("named-root deletion returned %d", status)
	}
	if _, err := os.Stat(filepath.Join(extra, "slot.sav")); !os.IsNotExist(err) {
		t.Fatalf("named-root file was not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(primary, "slot.sav")); err != nil {
		t.Fatalf("named-root deletion touched the primary save: %v", err)
	}
	status, _ = w.serveDeleteFile("/delete-file/"+game.ID, json.RawMessage(`{"root":"unknown","relPath":"slot.sav"}`), "")
	if status != 404 {
		t.Fatalf("unknown root returned %d, want 404", status)
	}
	lockedDir := filepath.Join(extra, "not-empty")
	if err := os.Mkdir(lockedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockedDir, "keep.sav"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, _ = w.serveDeleteFile("/delete-file/"+game.ID, json.RawMessage(`{"root":"config","relPath":"not-empty"}`), "")
	if status != 409 {
		t.Fatalf("failed deletion returned %d, want 409", status)
	}
	if _, err := os.Stat(filepath.Join(lockedDir, "keep.sav")); err != nil {
		t.Fatalf("failed deletion removed contents: %v", err)
	}
}
