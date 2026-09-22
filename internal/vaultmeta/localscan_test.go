package vaultmeta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

func TestScanLocalSummarizesTrackedAndDiscoveredSaves(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	extra := filepath.Join(root, "config")
	if err := os.MkdirAll(primary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, "save.dat"), []byte("save"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "settings.ini"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	if err := s.CreateGame(store.Game{ID: "game-a", Name: "Game A", SavePath: primary}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot("game-a", "config", extra); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSnapshot(store.Snapshot{
		ID: "snap-latest", GameID: "game-a", BranchName: "main",
		Timestamp: "2026-09-22T10:00:00Z", ZipPath: filepath.Join(root, "snap.zip"),
	}); err != nil {
		t.Fatal(err)
	}

	scan, err := ScanLocal(s, []presets.DiscoveredSave{
		{ID: "z", GroupID: "group", Name: "Unknown", Type: "game"},
		{ID: "a", GroupID: "group", Name: "Measured", Type: "game", Measured: true, FileCount: 3, TotalBytes: 42},
	})
	if err != nil {
		t.Fatalf("ScanLocal() error = %v", err)
	}
	if len(scan.Library.Games) != 1 {
		t.Fatalf("games = %#v", scan.Library.Games)
	}
	game := scan.Library.Games[0]
	if game.FileCount != 2 || game.TotalBytes != int64(len("save")+len("config")) {
		t.Fatalf("game counts = %#v", game)
	}
	if len(game.ContentHash) != 64 || game.SnapshotCount != 1 || game.LatestSnapshotID != "snap-latest" {
		t.Fatalf("game summary = %#v", game)
	}
	if game.HeadSnapshotID != "" || len(game.AncestorSnapshotIDs) != 0 {
		t.Fatalf("legacy snapshot order was incorrectly treated as ancestry: %#v", game)
	}
	if scan.Complete {
		t.Fatal("ScanLocal() marked an unmeasured candidate as complete")
	}
	if scan.Candidates[0].ID != "a" || scan.Candidates[1].ID != "z" {
		t.Fatalf("candidates = %#v", scan.Candidates)
	}
	// Candidate summaries are deliberately path-free; their JSON must not
	// disclose a local save location.
	encodedCandidates, err := json.Marshal(scan.Candidates)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedCandidates), primary) || strings.Contains(string(encodedCandidates), "savePath") {
		t.Fatalf("candidate summary leaked a local path: %s", encodedCandidates)
	}
}

func TestScanLocalFailsClosedForMissingTrackedLocation(t *testing.T) {
	root := t.TempDir()
	s := openTestStore(t, root)
	missing := filepath.Join(root, "missing")
	if err := s.CreateGame(store.Game{ID: "missing", Name: "Missing", SavePath: missing}); err != nil {
		t.Fatal(err)
	}

	_, err := ScanLocal(s, nil)
	if err == nil || !strings.Contains(err.Error(), "scan local game missing") {
		t.Fatalf("ScanLocal() error = %v, want explicit missing-location failure", err)
	}
}

func TestScanLocalFailsClosedForUnreadableExtraLocation(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	if err := os.MkdirAll(primary, 0o755); err != nil {
		t.Fatal(err)
	}
	s := openTestStore(t, root)
	if err := s.CreateGame(store.Game{ID: "game-a", Name: "Game A", SavePath: primary}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot("game-a", "config", filepath.Join(root, "missing-extra")); err != nil {
		t.Fatal(err)
	}

	_, err := ScanLocal(s, nil)
	if err == nil || !strings.Contains(err.Error(), "unreadable save locations") {
		t.Fatalf("ScanLocal() error = %v, want unreadable-extra failure", err)
	}
}

func openTestStore(t *testing.T, root string) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(root, "opensave.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
