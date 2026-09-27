package snapshot

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Fixed categories deliberately omit archive entry names and local paths.
var (
	ErrRestoreArchive  = errors.New("restore archive validation failed; nothing was changed")
	ErrRestoreLocation = errors.New("restore locations are missing or incompatible; nothing was changed")
	ErrRestoreSafety   = errors.New("restore safety snapshot creation or validation failed; nothing was changed")
)

// Read every entry before destructive extraction, not only the central index.
// This closes the legacy/local CRC-after-clearing bug. It does not prove that
// all current files were captured by the safety snapshot, or bound ZIP resources.
func verifyRestorePayload(zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return ErrRestoreArchive
	}
	defer r.Close()
	for _, entry := range r.File {
		if !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() {
			return ErrRestoreArchive
		}
		if entry.FileInfo().IsDir() && entry.UncompressedSize64 != 0 {
			return ErrRestoreArchive
		}
		src, err := entry.Open()
		if err != nil {
			return ErrRestoreArchive
		}
		n, readErr := io.Copy(io.Discard, src)
		closeErr := src.Close()
		if readErr != nil || closeErr != nil || uint64(n) != entry.UncompressedSize64 {
			return ErrRestoreArchive
		}
	}
	return nil
}

// Manager-level restore must be complete: the lower-level UnzipRoots contract
// still allows callers to inspect unplaced locations, but Restore may not
// silently omit one and report success. Existing extra-file roots are blocked:
// the legacy multi-root extractor only supports directory destinations there.
func preflightRestore(zipPath, primary string, extra map[string]string) error {
	if strings.TrimSpace(primary) == "" {
		return ErrRestoreLocation
	}
	if err := verifyRestorePayload(zipPath); err != nil {
		return err
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return ErrRestoreArchive
	}
	defer r.Close()
	info, err := os.Lstat(primary)
	if err != nil && !os.IsNotExist(err) {
		return ErrRestoreLocation
	}
	if err == nil && !info.IsDir() && !info.Mode().IsRegular() {
		return ErrRestoreLocation
	}
	primaryFile := err == nil && info.Mode().IsRegular()
	primaryEntries := 0
	for _, entry := range r.File {
		root, isRoot := rootOfEntry(entry.Name)
		if isRoot {
			path := extra[root]
			if strings.TrimSpace(path) == "" {
				return ErrRestoreLocation
			}
			if info, err := os.Lstat(path); err == nil {
				if !info.IsDir() {
					return ErrRestoreLocation
				}
			} else if !os.IsNotExist(err) {
				return ErrRestoreLocation
			}
		} else {
			primaryEntries++
			if primaryFile && (!entry.Mode().IsRegular() || strings.ContainsAny(entry.Name, "/\\")) {
				return ErrRestoreLocation
			}
		}
	}
	if primaryFile && primaryEntries != 1 {
		return ErrRestoreLocation
	}
	// Nested destinations would be cleared by another location during extraction.
	paths := []string{primary}
	seenRoots := map[string]bool{}
	for _, root := range r.File {
		if name, ok := rootOfEntry(root.Name); ok {
			if !seenRoots[name] {
				paths = append(paths, extra[name])
				seenRoots[name] = true
			}
		}
	}
	for i, a := range paths {
		for _, b := range paths[i+1:] {
			if restorePathsOverlap(a, b) {
				return ErrRestoreLocation
			}
		}
	}
	return nil
}

func restorePathsOverlap(a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		x, err := filepath.Abs(pair[0])
		if err != nil {
			return true
		}
		y, err := filepath.Abs(pair[1])
		if err != nil {
			return true
		}
		rel, err := filepath.Rel(x, y)
		if err != nil {
			continue
		} // different volumes
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
