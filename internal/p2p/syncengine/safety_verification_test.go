package syncengine

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncRejectsUnverifiedSafetyArchive(t *testing.T) {
	for _, kind := range []string{"corrupt", "readable-but-incomplete"} {
		t.Run(kind, func(t *testing.T) {
			env := setupEngine(t)
			write(t, env.localDir, "save.dat", "original")
			write(t, env.remoteDir, "save.dat", "original")
			if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err != nil {
				t.Fatal(err)
			}
			write(t, env.remoteDir, "save.dat", "remote replacement")
			env.engine.Snapshots.OnUpload = func(path, _ string) {
				if kind == "corrupt" {
					if err := os.WriteFile(path, []byte("not a ZIP"), 0600); err != nil {
						t.Fatal(err)
					}
					return
				}
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				z := zip.NewWriter(f)
				if _, err := z.Create("unrelated.sav"); err != nil {
					t.Fatal(err)
				}
				if err := z.Close(); err != nil {
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := env.engine.SyncWithPeer(context.Background(), "game1", env.peer); err == nil {
				t.Fatal("sync accepted an unverified safety snapshot")
			}
			got, err := os.ReadFile(filepath.Join(env.localDir, "save.dat"))
			if err != nil || string(got) != "original" {
				t.Fatalf("original save replaced: %q, %v", got, err)
			}
		})
	}
}
