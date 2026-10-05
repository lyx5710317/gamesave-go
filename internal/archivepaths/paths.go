// Package archivepaths validates snapshot extraction targets for all transports.
package archivepaths

import (
	"archive/zip"
	"errors"
	"os"
	"path"
	"strings"
)

var ErrUnsafePaths = errors.New("snapshot archive contains unsafe or conflicting paths")

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
func Validate(entries []*zip.File, windows bool) error {
	explicit := make(map[string]bool, len(entries))
	directories := make(map[string]bool)
	for _, entry := range entries {
		isDir := entry.FileInfo().IsDir()
		if !safeZipEntryName(entry.Name) || (entry.Mode().Type() != 0 && entry.Mode().Type() != os.ModeDir) ||
			isDir != strings.HasSuffix(entry.Name, "/") || (isDir && entry.UncompressedSize64 != 0) {
			return ErrUnsafePaths
		}
		key := strings.TrimSuffix(entry.Name, "/")
		if windows {
			for _, component := range strings.Split(key, "/") {
				if !SafeWindowsComponent(component) {
					return ErrUnsafePaths
				}
			}
			key = strings.ToUpper(key)
		}
		if _, exists := explicit[key]; exists || (!isDir && directories[key]) {
			return ErrUnsafePaths
		}
		explicit[key] = isDir
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if parentIsDir, exists := explicit[parent]; exists && !parentIsDir {
				return ErrUnsafePaths
			}
			directories[parent] = true
		}
	}
	return nil
}

func SafeWindowsComponent(name string) bool {
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
