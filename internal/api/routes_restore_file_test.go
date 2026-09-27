package api

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

func fileRestoreGame(t *testing.T) *testServer {
	t.Helper()
	ts := startTestServer(t)
	if err := ts.daemon.Store.CreateGame(store.Game{ID: "game", Name: "Synthetic", SavePath: ts.saveDir}); err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestRestoreFileSafetyFailureLeavesCurrentFile(t *testing.T) {
	ts := fileRestoreGame(t)
	path := filepath.Join(ts.saveDir, "slot.txt")
	os.WriteFile(path, []byte("target"), 0o600)
	snap, err := ts.daemon.Snapshots.Create("game", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("current"), 0o600)
	cfg, _ := ts.daemon.Store.GetSettings()
	blocker := filepath.Join(t.TempDir(), "blocked")
	os.WriteFile(blocker, []byte("not a directory"), 0o600)
	cfg.BackupsDir = blocker
	if err := ts.daemon.Store.UpdateSettings(cfg); err != nil {
		t.Fatal(err)
	}
	resp, _ := ts.do(t, http.MethodPost, "/api/games/game/snapshot/"+snap.ID+"/restore-file", map[string]string{"relPath": "slot.txt"})
	got, err := os.ReadFile(path)
	if resp.StatusCode == http.StatusOK || err != nil || string(got) != "current" {
		t.Fatalf("unsafe restore: status %d, current preserved %v", resp.StatusCode, string(got) == "current")
	}
}

func TestRestoreFileExtraLocationAndSafetyRecovery(t *testing.T) {
	ts := fileRestoreGame(t)
	config := t.TempDir()
	if err := ts.daemon.Store.AddGameRoot("game", "config", config); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "settings.ini")
	os.WriteFile(path, []byte("target"), 0o600)
	snap, err := ts.daemon.Snapshots.Create("game", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("current"), 0o600)
	rel := snapshot.RootPrefix + "config/settings.ini"
	resp, _ := ts.do(t, http.MethodPost, "/api/games/game/snapshot/"+snap.ID+"/restore-file", map[string]string{"relPath": rel})
	got, _ := os.ReadFile(path)
	if resp.StatusCode != http.StatusOK || string(got) != "target" {
		t.Fatalf("mapped restore failed: status %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(config, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatal("archive prefix was extracted into the location")
	}
	snaps, _ := ts.daemon.Store.ListSnapshots("game", "main")
	if len(snaps) != 2 {
		t.Fatalf("safety snapshot missing: %d", len(snaps))
	}
	if _, err := ts.daemon.Snapshots.Restore("game", snaps[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "current" {
		t.Fatal("pre-restore file was not recoverable")
	}
}

func TestRestoreFileSurvivesTargetRetention(t *testing.T) {
	ts := fileRestoreGame(t)
	g, _ := ts.daemon.Store.GetGame("game")
	g.MaxSnapshots = 1
	ts.daemon.Store.UpdateGame(g)
	path := filepath.Join(ts.saveDir, "slot.txt")
	os.WriteFile(path, []byte("target"), 0o600)
	snap, err := ts.daemon.Snapshots.Create("game", "target", true)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("current"), 0o600)
	resp, _ := ts.do(t, http.MethodPost, "/api/games/game/snapshot/"+snap.ID+"/restore-file", map[string]string{"relPath": "slot.txt"})
	got, _ := os.ReadFile(path)
	if resp.StatusCode != http.StatusOK || string(got) != "target" {
		t.Fatalf("target was lost during retention: status %d", resp.StatusCode)
	}
}

func TestRestoreFileRejectsUnverifiedSafetyOrChangedCurrentState(t *testing.T) {
	for _, kind := range []string{"broken-safety", "changed-current"} {
		t.Run(kind, func(t *testing.T) {
			ts := fileRestoreGame(t)
			path := filepath.Join(ts.saveDir, "slot.txt")
			if err := os.WriteFile(path, []byte("target"), 0o600); err != nil {
				t.Fatal(err)
			}
			snap, err := ts.daemon.Snapshots.Create("game", "target", false)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("current"), 0o600); err != nil {
				t.Fatal(err)
			}
			ts.daemon.Snapshots.OnUpload = func(archive, _ string) {
				if kind == "broken-safety" {
					if err := os.WriteFile(archive, []byte("broken safety archive"), 0o600); err != nil {
						t.Error(err)
					}
				} else {
					if err := os.WriteFile(path, []byte("concurrent change"), 0o600); err != nil {
						t.Error(err)
					}
				}
			}
			resp, body := ts.do(t, http.MethodPost, "/api/games/game/snapshot/"+snap.ID+"/restore-file", map[string]string{"relPath": "slot.txt"})
			want, code := "current", `"file_restore_safety"`
			if kind == "changed-current" {
				want, code = "concurrent change", `"file_restore_changed"`
			}
			got, _ := os.ReadFile(path)
			if resp.StatusCode != http.StatusConflict || string(body["code"]) != code || string(got) != want {
				t.Fatalf("safety failure was not preserved: %d", resp.StatusCode)
			}
		})
	}
}

func TestRestoreFileSingleFileMappingAndZeroLength(t *testing.T) {
	for _, payload := range []string{"remote bytes", ""} {
		name := "nonempty"
		if payload == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			ts := fileRestoreGame(t)
			g, err := ts.daemon.Store.GetGame("game")
			if err != nil {
				t.Fatal(err)
			}
			g.SavePath = filepath.Join(ts.saveDir, "local-name.txt")
			if err := os.WriteFile(g.SavePath, []byte("current"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ts.daemon.Store.UpdateGame(g); err != nil {
				t.Fatal(err)
			}
			remote := t.TempDir()
			file := filepath.Join(remote, "remote-name.txt")
			if err := os.WriteFile(file, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "target.zip")
			if _, err := snapshot.ZipPath(file, archive); err != nil {
				t.Fatal(err)
			}
			if err := ts.daemon.Store.CreateSnapshot(store.Snapshot{ID: "target", GameID: "game", BranchName: "main", ZipPath: archive}); err != nil {
				t.Fatal(err)
			}
			resp, _ := ts.do(t, http.MethodPost, "/api/games/game/snapshot/target/restore-file", map[string]string{"relPath": "remote-name.txt"})
			got, err := os.ReadFile(g.SavePath)
			if resp.StatusCode != http.StatusOK || err != nil || string(got) != payload {
				t.Fatalf("single-file mapping failed: %d", resp.StatusCode)
			}
			if _, err := os.Stat(filepath.Join(ts.saveDir, "remote-name.txt")); !os.IsNotExist(err) {
				t.Fatal("unmapped filename was published")
			}
		})
	}
}

func TestRestoreFilePublishFailureKeepsExistingDestination(t *testing.T) {
	staged := filepath.Join(t.TempDir(), "staged")
	if err := os.WriteFile(staged, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "destination")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dest, "current.txt")
	if err := os.WriteFile(marker, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishRestoredFile(staged, dest); err == nil {
		t.Fatal("publication replaced a directory")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "current" {
		t.Fatal("failed publication changed the destination")
	}
	parts, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".opensave-restore-publish-*.part"))
	if err != nil || len(parts) != 0 {
		t.Fatal("publication temp file leaked")
	}
}

func TestExtractSingleFileRejectsBadArchiveBeforeReplacing(t *testing.T) {
	kinds := []string{"crc", "duplicate", "directory", "symlink", "missing"}
	if runtime.GOOS == "windows" {
		kinds = append(kinds, "case-alias")
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: "slot.txt", Method: zip.Store}
			if kind == "directory" {
				h.Name += "/"
				h.SetMode(os.ModeDir | 0o755)
			}
			if kind == "symlink" {
				h.SetMode(os.ModeSymlink | 0o777)
			}
			f, _ := w.CreateHeader(h)
			f.Write([]byte("synthetic archive payload"))
			if kind == "duplicate" {
				f, _ = w.Create("slot.txt")
				f.Write([]byte("other"))
			}
			if kind == "case-alias" {
				f, _ = w.Create("SLOT.txt")
				f.Write([]byte("other"))
			}
			w.Close()
			data := b.Bytes()
			if kind == "crc" {
				data[bytes.Index(data, []byte("synthetic archive payload"))] ^= 1
			}
			archive := filepath.Join(t.TempDir(), "target.zip")
			os.WriteFile(archive, data, 0o600)
			dest := t.TempDir()
			path := filepath.Join(dest, "slot.txt")
			os.WriteFile(path, []byte("current"), 0o600)
			entry := "slot.txt"
			if kind == "directory" {
				entry += "/"
			}
			if kind == "missing" {
				entry = "absent.txt"
			}
			err := extractSingleFile(archive, entry, "slot.txt", dest)
			got, _ := os.ReadFile(path)
			if err == nil || string(got) != "current" {
				t.Fatal("invalid archive replaced the current file")
			}
			files, _ := os.ReadDir(dest)
			if len(files) != 1 {
				t.Fatal("staging file leaked")
			}
		})
	}
}

func TestSingleFileRestoreTargetRejectsUnsafeRelativePaths(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"../slot.txt", "sub/../slot.txt", "sub//slot.txt", "/slot.txt", "", ".", "sub/"} {
		if _, err := singleFileRestoreTarget(base, rel); err == nil {
			t.Fatalf("unsafe target accepted: %q", rel)
		}
	}
	if runtime.GOOS == "windows" {
		if _, err := singleFileRestoreTarget(base, "slot.txt:stream"); err == nil {
			t.Fatal("stream target accepted")
		}
	}
	dest, err := singleFileRestoreTarget(filepath.Join(base, "missing-folder"), "slot.txt")
	if err != nil || dest != filepath.Join(base, "missing-folder", "slot.txt") {
		t.Fatal("missing folder was guessed to be a file")
	}
}
