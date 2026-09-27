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
	"runtime"
	"strings"
)

// ErrLocalSnapshotConflict means a remote object has the same logical
// snapshot name as a different local archive. Neither copy is overwritten.
var ErrLocalSnapshotConflict = errors.New("local snapshot differs from remote; refusing to overwrite either copy")

// Never include untrusted entry names or archive contents in this error.
var ErrUnsafeSnapshotArchive = errors.New("cloud snapshot archive contains unsafe or conflicting paths; no saves were changed")

var ErrSnapshotSizeMismatch = errors.New("downloaded snapshot size mismatch")
var ErrSnapshotArchiveIntegrity = errors.New("cloud snapshot archive integrity check failed")

// RemoteVerification describes a read-only archive check. A ZIP with valid
// CRCs is readable, but without a trusted manifest or an identical local
// archive it is not proof of vault identity or snapshot ancestry.
type RemoteVerification struct {
	SizeBytes       int64  `json:"sizeBytes"`
	LocalComparison string `json:"localComparison"` // identical, different, unavailable
}

// VerifyRemoteSnapshot downloads one remote archive to an OS temporary file,
// checks ZIP structure and CRCs, and optionally compares its bytes with the
// local archive. It never publishes or restores the downloaded data.
func (s *Service) VerifyRemoteSnapshot(file CloudFile, localArchivePath string) (RemoteVerification, error) {
	if !safeSnapshotFileName(file.Name) || file.SizeBytes < 0 {
		return RemoteVerification{}, errors.New("invalid remote snapshot metadata")
	}
	tmp, err := os.CreateTemp("", ".opensave-cloud-verify-*.zip")
	if err != nil {
		return RemoteVerification{}, err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return RemoteVerification{}, err
	}
	defer os.Remove(tmpPath)
	if err := s.Download(file.Name, tmpPath); err != nil {
		return RemoteVerification{}, fmt.Errorf("download snapshot: %w", err)
	}
	actualSize, remoteHash, err := inspectSnapshotArchive(tmpPath, file.SizeBytes)
	if err != nil {
		return RemoteVerification{}, fmt.Errorf("verify downloaded snapshot: %w", err)
	}
	result := RemoteVerification{SizeBytes: actualSize, LocalComparison: "unavailable"}
	if localArchivePath == "" {
		return result, nil
	}
	info, err := os.Lstat(localArchivePath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return RemoteVerification{}, err
	}
	if !info.Mode().IsRegular() {
		return RemoteVerification{}, errors.New("local snapshot archive is not a regular file")
	}
	local, err := os.Open(localArchivePath)
	if err != nil {
		return RemoteVerification{}, err
	}
	defer local.Close()
	h := sha256.New()
	if _, err := io.Copy(h, local); err != nil {
		return RemoteVerification{}, err
	}
	if hex.EncodeToString(h.Sum(nil)) == remoteHash {
		result.LocalComparison = "identical"
	} else {
		result.LocalComparison = "different"
	}
	return result, nil
}

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
	_, actualHash, err := inspectSnapshotArchive(tmpPath, file.SizeBytes)
	if err != nil {
		return fmt.Errorf("verify downloaded snapshot: %w", err)
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

func inspectSnapshotArchive(zipPath string, expectedSize int64) (int64, string, error) {
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
		return 0, "", ErrSnapshotArchiveIntegrity
	}
	// Refuse inconsistent download metadata before hashing or decompressing.
	if expectedSize > 0 && info.Size() != expectedSize {
		return 0, "", fmt.Errorf("%w: got %d bytes, expected %d", ErrSnapshotSizeMismatch, info.Size(), expectedSize)
	}
	// Inspect and hash the same opened file, not a second path lookup.
	r, err := zip.NewReader(f, info.Size())
	if err != nil {
		return 0, "", ErrSnapshotArchiveIntegrity
	}
	if len(r.File) == 0 {
		return 0, "", ErrSnapshotArchiveIntegrity
	}
	if err := validateSnapshotArchivePaths(r.File, runtime.GOOS == "windows"); err != nil {
		return 0, "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", err
	}
	for _, entry := range r.File {
		reader, err := entry.Open()
		if err != nil {
			return 0, "", ErrSnapshotArchiveIntegrity
		}
		_, copyErr := io.Copy(io.Discard, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return 0, "", ErrSnapshotArchiveIntegrity
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
	trimmed := strings.TrimSuffix(name, "/")
	return trimmed != "" && trimmed != "." && trimmed != ".." &&
		path.Clean(trimmed) == trimmed && !strings.HasPrefix(trimmed, "../")
}

// Validate the whole tree before reading any entry. ZIP CRCs alone do not
// protect against duplicate extraction targets or a file used as a directory.
// Keep Linux case-sensitive behavior; Windows additionally rejects Win32
// device names, aliases and characters that cannot be restored safely.
func validateSnapshotArchivePaths(entries []*zip.File, windows bool) error {
	explicit := make(map[string]bool, len(entries))
	directories := make(map[string]bool)
	for _, entry := range entries {
		isDir := entry.FileInfo().IsDir()
		if !safeZipEntryName(entry.Name) || (entry.Mode().Type() != 0 && entry.Mode().Type() != os.ModeDir) ||
			isDir != strings.HasSuffix(entry.Name, "/") || (isDir && entry.UncompressedSize64 != 0) {
			return ErrUnsafeSnapshotArchive
		}
		key := strings.TrimSuffix(entry.Name, "/")
		if windows {
			for _, component := range strings.Split(key, "/") {
				if !safeWindowsArchiveComponent(component) {
					return ErrUnsafeSnapshotArchive
				}
			}
			key = strings.ToUpper(key)
		}
		if _, exists := explicit[key]; exists || (!isDir && directories[key]) {
			return ErrUnsafeSnapshotArchive
		}
		explicit[key] = isDir
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if parentIsDir, exists := explicit[parent]; exists && !parentIsDir {
				return ErrUnsafeSnapshotArchive
			}
			directories[parent] = true
		}
	}
	return nil
}

func safeWindowsArchiveComponent(name string) bool {
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, `<>"|?*`) {
		return false
	}
	for _, c := range name {
		if c < 32 {
			return false
		}
	}
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT") {
		suffix := stem[3:]
		if strings.Contains("123456789¹²³", suffix) && suffix != "" && len([]rune(suffix)) == 1 {
			return false
		}
	}
	return true
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
