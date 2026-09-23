package cloud

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrLocalSnapshotConflict means a remote object has the same logical
// snapshot name as a different local archive. Neither copy is overwritten.
var ErrLocalSnapshotConflict = errors.New("local snapshot differs from remote; refusing to overwrite either copy")

// DownloadVerified stages a remote snapshot next to its destination, verifies
// it, then publishes it without replacing a different local archive. A
// trusted SHA-256 may be supplied by a future vault manifest; existing cloud
// providers have only size metadata, so ZIP CRC validation is also required.
func (s *Service) DownloadVerified(file CloudFile, destPath, expectedSHA256 string) error {
	if !safeSnapshotFileName(file.Name) {
		return errors.New("invalid remote snapshot filename")
	}
	if expectedSHA256 != "" {
		decoded, err := hex.DecodeString(expectedSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return errors.New("invalid expected snapshot SHA-256")
		}
	}
	if file.SizeBytes < 0 {
		return errors.New("invalid remote snapshot size")
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o777); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".opensave-cloud-*.part")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)

	if err := s.Download(file.Name, tmpPath); err != nil {
		return fmt.Errorf("download snapshot: %w", err)
	}
	actualSize, actualHash, err := inspectSnapshotArchive(tmpPath)
	if err != nil {
		return fmt.Errorf("verify downloaded snapshot: %w", err)
	}
	if file.SizeBytes > 0 && actualSize != file.SizeBytes {
		return fmt.Errorf("downloaded snapshot size mismatch: got %d bytes, expected %d", actualSize, file.SizeBytes)
	}
	if expectedSHA256 != "" && !strings.EqualFold(actualHash, expectedSHA256) {
		return errors.New("downloaded snapshot SHA-256 mismatch")
	}
	return publishVerifiedArchive(tmpPath, destPath, actualHash)
}

func safeSnapshotFileName(name string) bool {
	return name != "" && strings.HasSuffix(name, ".zip") &&
		name != ".zip" && name != ".." &&
		!strings.Contains(name, "/") && !strings.Contains(name, "\\") &&
		!strings.Contains(name, ":") && !strings.ContainsRune(name, 0) &&
		filepath.Base(name) == name
}

func inspectSnapshotArchive(zipPath string) (int64, string, error) {
	f, err := os.Open(zipPath)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	if info.Size() == 0 {
		return 0, "", errors.New("snapshot archive is empty")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", err
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, "", err
	}
	defer r.Close()
	if len(r.File) == 0 {
		return 0, "", errors.New("snapshot archive has no entries")
	}
	for _, entry := range r.File {
		if !safeZipEntryName(entry.Name) || entry.Mode()&os.ModeSymlink != 0 {
			return 0, "", fmt.Errorf("unsafe snapshot archive entry %q", entry.Name)
		}
		reader, err := entry.Open()
		if err != nil {
			return 0, "", err
		}
		_, copyErr := io.Copy(io.Discard, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return 0, "", fmt.Errorf("snapshot archive entry %q: %w", entry.Name, copyErr)
		}
		if closeErr != nil {
			return 0, "", closeErr
		}
	}
	return info.Size(), hex.EncodeToString(h.Sum(nil)), nil
}

func safeZipEntryName(name string) bool {
	if name == "" || strings.Contains(name, "\\") || strings.Contains(name, ":") ||
		strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(name)
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

func publishVerifiedArchive(tmpPath, destPath, actualHash string) error {
	if _, err := os.Lstat(destPath); err == nil {
		return verifyExistingArchive(destPath, actualHash)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// A hard link publishes the already-verified file in one filesystem step
	// when supported, without ever replacing a file created concurrently.
	if err := os.Link(tmpPath, destPath); err == nil {
		return nil
	}
	// FAT/exFAT and some network filesystems do not support hard links. An
	// exclusive create remains no-overwrite; errors remove partial output.
	in, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return verifyExistingArchive(destPath, actualHash)
	}
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(destPath)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(destPath)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(destPath)
		return err
	}
	return nil
}

func verifyExistingArchive(destPath, actualHash string) error {
	info, err := os.Lstat(destPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrLocalSnapshotConflict
	}
	f, err := os.Open(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != actualHash {
		return ErrLocalSnapshotConflict
	}
	return nil
}
