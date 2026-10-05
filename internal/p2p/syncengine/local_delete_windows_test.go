//go:build windows

package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSyncFailsWhenLocalDeletionIsLocked(t *testing.T) {
	env := setupEngine(t)
	for _, dir := range []string{env.localDir, env.remoteDir} {
		write(t, dir, "save.dat", "original")
		write(t, dir, "retained.dat", "retained")
	}
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(env.remoteDir, "save.dat")); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(env.localDir, "save.dat"))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err == nil {
		t.Fatal("sync claimed success while deletion failed")
	}
	got, err := os.ReadFile(filepath.Join(env.localDir, "save.dat"))
	if err != nil || string(got) != "original" {
		t.Fatalf("locked save changed: %q, %v", got, err)
	}
}
