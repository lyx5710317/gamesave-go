package snapshot

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func replaceSafetyWithReadableZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, contents := range entries {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRejectsReadableButIncompleteSafety(t *testing.T) {
	for _, kind := range []string{"missing", "wrong-bytes", "empty-file-omitted", "extra-location-omitted", "empty-directory-omitted", "unexpected-entry"} {
		t.Run(kind, func(t *testing.T) {
			env := setup(t)
			config := t.TempDir()
			if err := env.store.AddGameRoot("game1", "config", config); err != nil {
				t.Fatal(err)
			}
			writeSave(t, env.saveDir, "slot.sav", "target")
			writeSave(t, config, "settings.ini", "target config")
			target, err := env.mgr.Create("game1", "target", false)
			if err != nil {
				t.Fatal(err)
			}
			writeSave(t, env.saveDir, "slot.sav", "current")
			writeSave(t, env.saveDir, "empty.sav", "")
			writeSave(t, config, "settings.ini", "current config")
			if err := os.Mkdir(filepath.Join(env.saveDir, "empty-dir"), 0o700); err != nil {
				t.Fatal(err)
			}
			env.mgr.OnUpload = func(path, _ string) {
				entries := map[string]string{"slot.sav": "current", "empty.sav": "", "empty-dir/": "", RootPrefix + "config/": "", RootPrefix + "config/settings.ini": "current config"}
				switch kind {
				case "missing":
					delete(entries, "slot.sav")
				case "wrong-bytes":
					entries["slot.sav"] = "changed"
				case "empty-file-omitted":
					delete(entries, "empty.sav")
				case "extra-location-omitted":
					delete(entries, RootPrefix+"config/settings.ini")
				case "empty-directory-omitted":
					delete(entries, "empty-dir/")
				case "unexpected-entry":
					entries["not-in-current.sav"] = "unexpected"
				}
				replaceSafetyWithReadableZip(t, path, entries)
			}
			if _, err := env.mgr.Restore("game1", target.ID); !errors.Is(err, ErrRestoreSafety) {
				t.Fatalf("unprotected restore was accepted: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav")); err != nil || string(got) != "current" {
				t.Fatal("current file replaced")
			}
			if got, err := os.ReadFile(filepath.Join(config, "settings.ini")); err != nil || string(got) != "current config" {
				t.Fatal("extra location replaced")
			}
			if _, err := os.Stat(filepath.Join(env.saveDir, "empty-dir")); err != nil {
				t.Fatal("empty directory lost")
			}
		})
	}
}

func TestRestoreStopsOnObservedCurrentTreeChange(t *testing.T) {
	for _, kind := range []string{"edit", "add", "delete", "directory"} {
		t.Run(kind, func(t *testing.T) {
			env := setup(t)
			writeSave(t, env.saveDir, "slot.sav", "target")
			target, err := env.mgr.Create("game1", "target", false)
			if err != nil {
				t.Fatal(err)
			}
			writeSave(t, env.saveDir, "slot.sav", "current")
			env.mgr.OnUpload = func(_, _ string) {
				switch kind {
				case "edit":
					writeSave(t, env.saveDir, "slot.sav", "concurrent")
				case "add":
					writeSave(t, env.saveDir, "new.sav", "concurrent")
				case "delete":
					if err := os.Remove(filepath.Join(env.saveDir, "slot.sav")); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(filepath.Join(env.saveDir, "new-dir"), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := env.mgr.Restore("game1", target.ID); !errors.Is(err, ErrRestoreChanged) {
				t.Fatalf("observed current change was not rejected: %v", err)
			}
			if kind == "delete" {
				if _, err := os.Stat(filepath.Join(env.saveDir, "slot.sav")); !os.IsNotExist(err) {
					t.Fatal("deleted file re-created")
				}
				return
			}
			want := "current"
			if kind == "edit" {
				want = "concurrent"
			}
			if got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav")); err != nil || string(got) != want {
				t.Fatal("current state overwritten")
			}
			if kind == "add" {
				if got, err := os.ReadFile(filepath.Join(env.saveDir, "new.sav")); err != nil || string(got) != "concurrent" {
					t.Fatal("new file lost")
				}
			}
			if kind == "directory" {
				if info, err := os.Stat(filepath.Join(env.saveDir, "new-dir")); err != nil || !info.IsDir() {
					t.Fatal("new directory lost")
				}
			}
		})
	}
}

func TestRestoreVerifiedWholeSafetyCanRecoverAllLocations(t *testing.T) {
	env := setup(t)
	config := t.TempDir()
	if err := env.store.AddGameRoot("game1", "config", config); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot.sav", "target")
	writeSave(t, config, "settings.ini", "target config")
	target, err := env.mgr.Create("game1", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot.sav", "current")
	writeSave(t, env.saveDir, ".hidden", "private state")
	writeSave(t, env.saveDir, "empty.sav", "")
	writeSave(t, config, "settings.ini", "current config")
	if err := os.Mkdir(filepath.Join(env.saveDir, "empty-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := env.mgr.Restore("game1", target.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav")); err != nil || string(got) != "target" {
		t.Fatal("target was not restored")
	}
	snaps, err := env.store.ListSnapshots("game1", "main")
	if err != nil {
		t.Fatal(err)
	}
	var safetyID string
	for _, snap := range snaps {
		if snap.IsSystemAuto {
			safetyID = snap.ID
			break
		}
	}
	if safetyID == "" {
		t.Fatal("verified safety snapshot not found")
	}
	if _, err := env.mgr.Restore("game1", safetyID); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		filepath.Join(env.saveDir, "slot.sav"):  "current",
		filepath.Join(env.saveDir, ".hidden"):   "private state",
		filepath.Join(env.saveDir, "empty.sav"): "",
		filepath.Join(config, "settings.ini"):   "current config",
	} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("safety recovery differs at %s: %v", path, err)
		}
	}
	if info, err := os.Stat(filepath.Join(env.saveDir, "empty-dir")); err != nil || !info.IsDir() {
		t.Fatal("empty directory was not recoverable")
	}
}
