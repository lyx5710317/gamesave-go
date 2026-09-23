package cloud

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

type downloadTestProvider struct {
	data  []byte
	err   error
	calls int
}

func (p *downloadTestProvider) Upload(string, string) error { return errors.New("not used") }
func (p *downloadTestProvider) List() ([]CloudFile, error)  { return nil, errors.New("not used") }
func (p *downloadTestProvider) Delete(CloudFile) error      { return errors.New("not used") }
func (p *downloadTestProvider) Download(_ string, localPath string) error {
	p.calls++
	if err := os.WriteFile(localPath, p.data, 0o600); err != nil {
		return err
	}
	return p.err
}

func newDownloadTestService(t *testing.T, provider *downloadTestProvider) *Service {
	t.Helper()
	svc, db := newTestService(t)
	if err := svc.RegisterProvider("test_download", provider); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "test_download"
	})
	return svc
}

func testZIP(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	entry, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertNoPartFiles(t *testing.T, dir string) {
	t.Helper()
	parts, err := filepath.Glob(filepath.Join(dir, ".opensave-cloud-*.part"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Fatalf("staging files remain: %v", parts)
	}
}

func TestDownloadVerifiedPublishesAndReusesIdenticalArchive(t *testing.T) {
	data := testZIP(t, "slot.sav", "saved progress")
	provider := &downloadTestProvider{data: data}
	svc := newDownloadTestService(t, provider)
	dir := t.TempDir()
	dest := filepath.Join(dir, "snap_1.zip")
	file := CloudFile{Name: "game__main__snap_1.zip", SizeBytes: int64(len(data))}
	for i := 0; i < 2; i++ {
		if err := svc.DownloadVerified(file, dest, testHash(data)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("published archive differs: %v", err)
		}
		assertNoPartFiles(t, dir)
	}
	if provider.calls != 2 {
		t.Fatalf("download calls = %d, want 2", provider.calls)
	}
}

func TestDownloadVerifiedRejectsFailuresWithoutPublishing(t *testing.T) {
	valid := testZIP(t, "slot.sav", "progress")
	unsafe := testZIP(t, "../outside.sav", "progress")
	for _, tc := range []struct {
		name, fileName, expectedHash string
		data                         []byte
		size                         int64
		providerErr                  error
	}{
		{"short download", "remote.zip", "", valid[:len(valid)/2], int64(len(valid)), nil},
		{"valid zip wrong size", "remote.zip", "", valid, int64(len(valid) + 1), nil},
		{"wrong hash", "remote.zip", strings.Repeat("0", 64), valid, int64(len(valid)), nil},
		{"invalid hash", "remote.zip", "not-a-hash", valid, int64(len(valid)), nil},
		{"corrupt zip", "remote.zip", "", []byte("not a zip"), 9, nil},
		{"unsafe zip entry", "remote.zip", "", unsafe, int64(len(unsafe)), nil},
		{"interrupted transfer", "remote.zip", "", valid[:len(valid)/2], 0, errors.New("connection lost")},
		{"path traversal", "../remote.zip", "", valid, int64(len(valid)), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &downloadTestProvider{data: tc.data, err: tc.providerErr}
			svc := newDownloadTestService(t, provider)
			dir := t.TempDir()
			dest := filepath.Join(dir, "snap_1.zip")
			if err := svc.DownloadVerified(CloudFile{Name: tc.fileName, SizeBytes: tc.size}, dest, tc.expectedHash); err == nil {
				t.Fatal("unsafe download succeeded")
			}
			if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe download published a file: %v", err)
			}
			assertNoPartFiles(t, dir)
		})
	}
}

func TestDownloadVerifiedDoesNotReplaceNonregularLocalTarget(t *testing.T) {
	data := testZIP(t, "slot.sav", "progress")
	svc := newDownloadTestService(t, &downloadTestProvider{data: data})
	dir := t.TempDir()
	dest := filepath.Join(dir, "snap_1.zip")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := svc.DownloadVerified(CloudFile{Name: "remote.zip", SizeBytes: int64(len(data))}, dest, ""); !errors.Is(err, ErrLocalSnapshotConflict) {
		t.Fatalf("nonregular local target = %v, want conflict", err)
	}
	info, err := os.Lstat(dest)
	if err != nil || !info.IsDir() {
		t.Fatalf("local target changed: %v", err)
	}
	assertNoPartFiles(t, dir)
}

func TestDownloadVerifiedPreservesDifferentLocalArchive(t *testing.T) {
	local := testZIP(t, "slot.sav", "local")
	remote := testZIP(t, "slot.sav", "remote")
	provider := &downloadTestProvider{data: remote}
	svc := newDownloadTestService(t, provider)
	dir := t.TempDir()
	dest := filepath.Join(dir, "snap_1.zip")
	if err := os.WriteFile(dest, local, 0o600); err != nil {
		t.Fatal(err)
	}
	err := svc.DownloadVerified(CloudFile{Name: "game__main__snap_1.zip", SizeBytes: int64(len(remote))}, dest, "")
	if !errors.Is(err, ErrLocalSnapshotConflict) {
		t.Fatalf("conflicting archive = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, local) {
		t.Fatalf("local archive changed: %v", err)
	}
	assertNoPartFiles(t, dir)
}
