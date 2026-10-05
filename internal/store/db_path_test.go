package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenLiteralDatabasePath(t *testing.T) {
	names := []string{"profile#fragment", "profile%23literal", "中文 空格 & 参数"}
	if runtime.GOOS != "windows" {
		names = append(names, "profile?_pragma=foreign_keys(0)")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), name)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "opensave.db")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.EnsureDefaultSettings("data", "backups"); err != nil {
				s.Close()
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("database not created at literal path: %v", err)
			}
			s, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			settings, err := s.GetSettings()
			if err != nil || settings.DataDir != "data" {
				t.Fatalf("reopen lost settings: %v", err)
			}
		})
	}
}

func TestOpenMemoryDatabaseKeepsPragmas(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var enabled int
	if err := s.db.Get(&enabled, "PRAGMA foreign_keys"); err != nil || enabled != 1 {
		t.Fatalf("foreign keys disabled: %d, %v", enabled, err)
	}
}
