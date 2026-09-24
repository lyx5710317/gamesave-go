package p2p

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

func newMatchTestEngine(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "opensave.db"))
	if err != nil {
		t.Fatalf("store.Open error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatalf("EnsureDefaultSettings error = %v", err)
	}
	return &Engine{Store: s, Log: func(string, string) {}}, s
}

func TestTemporaryAutoTrackPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Users\Alice\AppData\Local\Temp\test-saves`, true},
		{`c:/users/alice/appdata/local/temp/test-saves`, true},
		{`C:\Windows\Temp\game`, true},
		{`/tmp/game`, true},
		{`/var/tmp/game`, true},
		{`/private/tmp/game`, true},
		{`C:\Users\Alice\AppData\Local\Tempest\game`, false},
		{`C:\Users\Alice\Saved Games\game`, false},
		{`C:\game save`, false},
		{`/home/alice/.local/share/game`, false},
	}
	for _, tt := range tests {
		if got := temporaryAutoTrackPath(tt.path); got != tt.want {
			t.Errorf("temporaryAutoTrackPath(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}

func TestEnsureManifestGameRejectsGuessedTemporaryPath(t *testing.T) {
	e, s := newMatchTestEngine(t)
	peerPath := `C:\Users\Alice\AppData\Local\Temp\test-saves`
	_, err := e.ensureManifestGame("vm-test", manifestGameQuery{Name: "VM Test", SavePath: peerPath})
	if err == nil || !strings.Contains(err.Error(), "temporary save path") {
		t.Fatalf("ensureManifestGame error = %v, want actionable temporary-path error", err)
	}
	if _, err := s.GetGame("vm-test"); err == nil {
		t.Fatal("temporary peer path must not be auto-tracked")
	}

	// A user-supplied translation to a real local folder is explicit intent.
	settings, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(t.TempDir(), "game-save")
	settings.PathTranslations = []store.TranslationRule{{FromPattern: peerPath, ToPattern: localPath}}
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	game, err := e.ensureManifestGame("vm-test", manifestGameQuery{Name: "VM Test", SavePath: peerPath})
	if err != nil {
		t.Fatalf("explicit translation should permit tracking: %v", err)
	}
	if game.SavePath != localPath {
		t.Errorf("translated save path = %q, want %q", game.SavePath, localPath)
	}
}

func TestEnsureManifestGameKeepsExistingSameMachineTemporaryPath(t *testing.T) {
	e, _ := newMatchTestEngine(t)
	// The existing same-machine case is used by the end-to-end test daemons:
	// both peers can see this exact directory, so no destination is guessed.
	savePath := t.TempDir()
	game, err := e.ensureManifestGame("same-machine-test", manifestGameQuery{
		Name: "Same Machine Test", SavePath: savePath,
	})
	if err != nil {
		t.Fatalf("existing unchanged temporary path should remain trackable: %v", err)
	}
	if !strings.EqualFold(filepath.Clean(game.SavePath), filepath.Clean(savePath)) {
		t.Errorf("save path = %q, want %q", game.SavePath, savePath)
	}

	missing := filepath.Join(t.TempDir(), "not-yet-created")
	if _, err := e.ensureManifestGame("missing-temporary-test", manifestGameQuery{
		Name: "Missing Temporary Test", SavePath: missing,
	}); err == nil {
		t.Fatal("a missing temporary destination must require an explicit path")
	}
}

// TestEnsureManifestGameMatching pins the peer-game resolution order:
// explicit alias and (opt-in) App ID both link a peer's differently-named
// game to the local canonical one, while the App ID path stays inert until
// the user turns it on — so a cracked and a legit copy never merge silently.
func TestEnsureManifestGameMatching(t *testing.T) {
	e, s := newMatchTestEngine(t)

	if err := s.CreateGame(store.Game{
		ID: "nevergrave", Name: "NeverGrave",
		SavePath: filepath.Join(t.TempDir(), "steam"), AppID: "2069710", ActiveBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}

	// Matching OFF (default): a peer game with the same App ID but a
	// different id must NOT resolve to the local game. With no name/path to
	// auto-track, it's simply reported not-found here.
	if _, err := e.ensureManifestGame("nevergrave-cracked", manifestGameQuery{AppID: "2069710"}); err == nil {
		t.Error("with App ID matching off, a shared App ID should not resolve to the local game")
	}

	// Turn App ID matching on.
	set, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	set.MatchByAppID = true
	if err := s.UpdateSettings(set); err != nil {
		t.Fatal(err)
	}

	// Now the same shared App ID resolves to the canonical game.
	g, err := e.ensureManifestGame("nevergrave-cracked", manifestGameQuery{AppID: "2069710"})
	if err != nil {
		t.Fatalf("ensureManifestGame (App ID on) error = %v", err)
	}
	if g.ID != "nevergrave" {
		t.Errorf("App ID match resolved to %q, want nevergrave", g.ID)
	}

	// An explicit link resolves regardless of App ID or the toggle.
	if err := s.AddGameAlias("some-portable-id", "nevergrave"); err != nil {
		t.Fatal(err)
	}
	g, err = e.ensureManifestGame("some-portable-id", manifestGameQuery{})
	if err != nil {
		t.Fatalf("ensureManifestGame (alias) error = %v", err)
	}
	if g.ID != "nevergrave" {
		t.Errorf("alias resolved to %q, want nevergrave", g.ID)
	}
}
