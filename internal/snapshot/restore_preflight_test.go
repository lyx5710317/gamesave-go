package snapshot

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreRejectsOverlappingLocations(t *testing.T) {
	for _, kind := range []string{"nested", "identical"} {
		t.Run(kind, func(t *testing.T) {
			env := setup(t)
			config := t.TempDir()
			if err := env.store.AddGameRoot("game1", "config", config); err != nil {
				t.Fatal(err)
			}
			writeSave(t, env.saveDir, "slot.sav", "archived state")
			writeSave(t, config, "settings.ini", "archived config")
			target, err := env.mgr.Create("game1", "target", false)
			if err != nil {
				t.Fatal(err)
			}
			path := env.saveDir
			if kind == "nested" {
				path = filepath.Join(env.saveDir, "config")
			}
			writeSave(t, env.saveDir, "slot.sav", "keep current state")
			// Store insertion already rejects this topology. Exercise the
			// restore boundary directly for legacy/imported mappings instead.
			if err := preflightRestore(target.ZipPath, env.saveDir, map[string]string{"config": path}); !errors.Is(err, ErrRestoreLocation) {
				t.Fatalf("bad mapping accepted: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav"))
			if err != nil || string(got) != "keep current state" {
				t.Fatalf("live state changed: %v", err)
			}
		})
	}
}

func TestRestoreRejectsCorruptedSafetyArchive(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot.sav", "archived state")
	target, err := env.mgr.Create("game1", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot.sav", "keep current state")
	env.mgr.OnUpload = func(path, _ string) {
		if err := os.WriteFile(path, []byte("broken synthetic zip"), 0o600); err != nil {
			t.Error(err)
		}
	}
	if _, err := env.mgr.Restore("game1", target.ID); !errors.Is(err, ErrRestoreSafety) {
		t.Fatalf("invalid safety accepted: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav"))
	if err != nil || string(got) != "keep current state" {
		t.Fatalf("live state changed: %v", err)
	}
}

func TestRestoreRejectsCorruptPayloadBeforeClearing(t *testing.T) {
	env := setup(t)
	writeSave(t, env.saveDir, "slot.sav", "unique archived payload")
	target, err := env.mgr.Create("game1", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(data, []byte("unique archived payload"))
	if i < 0 {
		t.Fatal("missing synthetic payload")
	}
	data[i] ^= 1
	if err := os.WriteFile(target.ZipPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot.sav", "current protected state")
	writeSave(t, env.saveDir, "other.sav", "keep me")
	if _, err := env.mgr.Restore("game1", target.ID); err == nil {
		t.Fatal("corrupt restore succeeded")
	}
	for name, want := range map[string]string{"slot.sav": "current protected state", "other.sav": "keep me"} {
		got, err := os.ReadFile(filepath.Join(env.saveDir, name))
		if err != nil || string(got) != want {
			t.Fatalf("live file changed: %s, %v", name, err)
		}
	}
}

func TestRestoreRejectsUnmappedLocationBeforeClearing(t *testing.T) {
	env := setup(t)
	config := t.TempDir()
	if err := env.store.AddGameRoot("game1", "config", config); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot.sav", "archived state")
	writeSave(t, config, "settings.ini", "archived config")
	target, err := env.mgr.Create("game1", "target", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.RemoveGameRoot("game1", "config"); err != nil {
		t.Fatal(err)
	}
	writeSave(t, env.saveDir, "slot.sav", "keep current state")
	if _, err := env.mgr.Restore("game1", target.ID); err == nil {
		t.Fatal("partial restore reported success")
	}
	got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav"))
	if err != nil || string(got) != "keep current state" {
		t.Fatalf("live primary changed: %v", err)
	}
}

func TestUnzipToMissingFolderKeepsFilesInsideConfiguredPath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeSave(t, source, "slot.sav", "snapshot")
	archive := filepath.Join(root, "source.zip")
	if _, err := ZipPath(source, archive); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "missing-folder")
	if err := UnzipTo(archive, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "slot.sav"))
	if err != nil || string(got) != "snapshot" {
		t.Fatalf("not restored inside configured folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "slot.sav")); !os.IsNotExist(err) {
		t.Fatal("archive scattered into parent")
	}
}

func TestUnzipToExistingFileMapsArchiveBasenameAndRejectsMultiple(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		label := "single"
		if multiple {
			label = "multiple"
		}
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "source.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(f)
			e, _ := w.Create("remote.sav")
			_, _ = e.Write([]byte("snapshot"))
			if multiple {
				e, _ = w.Create("another.sav")
				_, _ = e.Write([]byte("second"))
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(root, "local.sav")
			if err := os.WriteFile(dest, []byte("current"), 0o600); err != nil {
				t.Fatal(err)
			}
			err = UnzipTo(archive, dest)
			want := "snapshot"
			if multiple {
				want = "current"
				if err == nil {
					t.Fatal("multiple files accepted for file target")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != want {
				t.Fatalf("file target incorrect: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "remote.sav")); !os.IsNotExist(err) {
				t.Fatal("archive basename leaked to sibling")
			}
		})
	}
}
