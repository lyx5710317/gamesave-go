package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalRestoreRejectsUnsafePathsBeforeClearing(t *testing.T) {
	cases := []map[string]string{
		{"folder/../slot.sav": "incoming"},
		{"./slot.sav": "incoming"},
		{"../escape.sav": "incoming"},
		{"folder": "file", "folder/slot.sav": "child"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, map[string]string{"Slot.sav": "one", "slot.sav": "two"}, map[string]string{"NUL.sav": "incoming"})
	}
	for _, entries := range cases {
		env := setup(t)
		writeSave(t, env.saveDir, "slot.sav", "original")
		target, err := env.mgr.Create("game1", "target", false)
		if err != nil {
			t.Fatal(err)
		}
		replaceSafetyWithReadableZip(t, target.ZipPath, entries)
		if _, err := env.mgr.Restore("game1", target.ID); !errors.Is(err, ErrRestoreArchive) {
			t.Errorf("unsafe local archive accepted: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(env.saveDir, "slot.sav"))
		if err != nil || string(got) != "original" {
			t.Errorf("live save changed before rejecting unsafe paths: %q, %v", got, err)
		}
	}
}
