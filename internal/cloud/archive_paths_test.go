package cloud

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func zipWithPaths(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if name[len(name)-1] != '/' {
			if _, err := f.Write([]byte("synthetic test data")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCloudArchiveRejectsAmbiguousPathsBeforePublishing(t *testing.T) {
	cases := [][]string{
		{"slot.sav", "slot.sav"},
		{"folder/../slot.sav"},
		{"./slot.sav"},
		{"folder//slot.sav"},
		{"folder", "folder/slot.sav"},
		{"folder/slot.sav", "folder"},
		{"folder/", "folder"},
		{"folder/", "folder/"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, []string{"Slot.sav", "slot.sav"}, []string{"NUL.sav"}, []string{"folder /slot.sav"})
	}
	for _, names := range cases {
		t.Run(names[0], func(t *testing.T) {
			data := zipWithPaths(t, names...)
			svc := newDownloadTestService(t, &downloadTestProvider{data: data})
			dir := t.TempDir()
			dest := filepath.Join(dir, "snap.zip")
			file := CloudFile{Name: "game__main__snap.zip", SizeBytes: int64(len(data))}
			if err := svc.DownloadVerified(file, dest, ""); !errors.Is(err, ErrUnsafeSnapshotArchive) {
				t.Fatalf("ambiguous archive result = %v", err)
			}
			if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("archive was published: %v", err)
			}
			assertNoPartFiles(t, dir)
			if _, err := svc.VerifyRemoteSnapshot(file, ""); !errors.Is(err, ErrUnsafeSnapshotArchive) {
				t.Fatalf("ambiguous verification result = %v", err)
			}
		})
	}
}

func TestArchivePathValidationPreservesOrdinaryAndMultiLocationSnapshots(t *testing.T) {
	for _, names := range [][]string{
		{"slot.sav"},
		{"empty/", "保存/", "保存/进度.sav"},
		{"folder/slot.sav", "folder/"}, // explicit directory after implicit parent
		{"primary.sav", ".opensave-locations/config/", ".opensave-locations/config/settings.ini"},
	} {
		data := zipWithPaths(t, names...)
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		for _, windows := range []bool{false, true} {
			if err := validateSnapshotArchivePaths(r.File, windows); err != nil {
				t.Fatalf("ordinary archive %v, windows=%v: %v", names, windows, err)
			}
		}
	}
}

func TestArchivePathsWindowsPolicyDoesNotChangeLinuxCaseSensitivity(t *testing.T) {
	for _, names := range [][]string{{"Slot.sav", "slot.sav"}, {"NUL.sav"}, {"folder /slot.sav"}} {
		data := zipWithPaths(t, names...)
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSnapshotArchivePaths(r.File, false); err != nil {
			t.Fatalf("Linux path rejected: %v", err)
		}
		if err := validateSnapshotArchivePaths(r.File, true); !errors.Is(err, ErrUnsafeSnapshotArchive) {
			t.Fatalf("Windows alias accepted: %v", err)
		}
	}
}

func TestWindowsArchiveNames(t *testing.T) {
	for _, name := range []string{"NUL.sav", "con", "AUX.tar.gz", "COM1", "LPT9.sav", "COM¹", "LPT².dat", "con .sav", "CONOUT$", "slot.", "slot ", "a?b", "a*b", "a|b", "a\x01b"} {
		if safeWindowsArchiveComponent(name) {
			t.Fatalf("unsafe Windows component accepted: %q", name)
		}
	}
	for _, name := range []string{"slot.sav", ".opensave-locations", "进度", "COM10.sav", "LPT0", "CONSOLE", "a b"} {
		if !safeWindowsArchiveComponent(name) {
			t.Fatalf("ordinary Windows component rejected: %q", name)
		}
	}
}

func TestArchivePathValidationRejectsSpecialModesAndDirectoryBodies(t *testing.T) {
	for _, mode := range []os.FileMode{os.ModeSymlink, os.ModeNamedPipe, os.ModeDevice, os.ModeSocket} {
		for _, name := range []string{"slot.sav", "folder/"} {
			header := zip.FileHeader{Name: name}
			header.SetMode(mode | 0o600)
			if err := validateSnapshotArchivePaths([]*zip.File{{FileHeader: header}}, false); !errors.Is(err, ErrUnsafeSnapshotArchive) {
				t.Fatalf("special entry mode %v accepted: %v", mode, err)
			}
		}
	}
	header := zip.FileHeader{Name: "folder/", UncompressedSize64: 1}
	if err := validateSnapshotArchivePaths([]*zip.File{{FileHeader: header}}, false); !errors.Is(err, ErrUnsafeSnapshotArchive) {
		t.Fatalf("directory with hidden body accepted: %v", err)
	}
}

func TestArchiveSizeMismatchIsCheckedBeforeZIPParsing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "invalid.zip")
	if err := os.WriteFile(p, []byte("not a ZIP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectSnapshotArchive(p, 100); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("size mismatch not checked first: %v", err)
	}
}

func TestCloudArchiveCRCFailureDoesNotExposeEntryNameOrPublish(t *testing.T) {
	const privateName = "private-save-name.sav"
	data := zipWithPaths(t, privateName)
	index := bytes.Index(data, []byte("synthetic test data"))
	if index < 0 {
		t.Fatal("test payload missing")
	}
	data[index] ^= 1
	svc := newDownloadTestService(t, &downloadTestProvider{data: data})
	dir := t.TempDir()
	err := svc.DownloadVerified(CloudFile{Name: "game__main__snap.zip", SizeBytes: int64(len(data))}, filepath.Join(dir, "snap.zip"), "")
	if err == nil || strings.Contains(err.Error(), privateName) {
		t.Fatalf("CRC validation/privacy failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "snap.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt archive published: %v", err)
	}
	assertNoPartFiles(t, dir)
}
