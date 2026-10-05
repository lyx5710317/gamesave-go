package snapshot

import (
	"archive/zip"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/opensave/opensave/internal/archivepaths"
)

// Fixed categories deliberately omit archive entry names and local paths.
var (
	ErrRestoreArchive  = errors.New("restore archive validation failed; nothing was changed")
	ErrRestoreLocation = errors.New("restore locations are missing or incompatible; nothing was changed")
	ErrRestoreSafety   = errors.New("restore safety snapshot creation or validation failed; nothing was changed")
	ErrRestoreChanged  = errors.New("current save changed during restore preparation; nothing was replaced")
)

type restoreFingerprint struct {
	Directory bool
	Size      int64
	Hash      [sha256.Size]byte
}

type restoreCurrentState struct {
	Entries map[string]restoreFingerprint
	Kinds   map[string]string
}

// Read every entry before destructive extraction, not only the central index.
func verifyRestorePayload(zipPath string) error {
	_, err := restoreArchiveInventory(zipPath)
	return err
}

// Hash actual archive bytes, not captured-file database rows, and consume EOF
// so CRC checks run. Duplicate names cannot prove a unique captured state.
func restoreArchiveInventory(zipPath string) (map[string]restoreFingerprint, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, ErrRestoreArchive
	}
	defer r.Close()
	if err := archivepaths.Validate(r.File, runtime.GOOS == "windows"); err != nil {
		return nil, ErrRestoreArchive
	}
	entries := map[string]restoreFingerprint{}
	for _, entry := range r.File {
		if _, exists := entries[entry.Name]; exists {
			return nil, ErrRestoreArchive
		}
		if !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() {
			return nil, ErrRestoreArchive
		}
		if entry.FileInfo().IsDir() && entry.UncompressedSize64 != 0 {
			return nil, ErrRestoreArchive
		}
		src, err := entry.Open()
		if err != nil {
			return nil, ErrRestoreArchive
		}
		sum := sha256.New()
		n, readErr := io.Copy(sum, src)
		closeErr := src.Close()
		if readErr != nil || closeErr != nil || uint64(n) != entry.UncompressedSize64 {
			return nil, ErrRestoreArchive
		}
		fp := restoreFingerprint{Directory: entry.FileInfo().IsDir(), Size: n}
		if !fp.Directory {
			copy(fp.Hash[:], sum.Sum(nil))
		}
		entries[entry.Name] = fp
	}
	return entries, nil
}

// A read-only restore verification scan, matching the existing ZIP root layout.
// Unlike a normal best-effort snapshot, unreadable or special files stop it.
// Include excluded/dot files and empty directories: clearing removes them too.
func readRestoreCurrentState(primary string, extra map[string]string) (restoreCurrentState, error) {
	state := restoreCurrentState{Entries: map[string]restoreFingerprint{}, Kinds: map[string]string{}}
	roots := map[string]string{"": primary}
	for name, path := range extra {
		roots[name] = path
	}
	for root, path := range roots {
		if strings.TrimSpace(path) == "" {
			state.Kinds[root] = "unmapped"
			continue
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			state.Kinds[root] = "missing"
			continue
		}
		if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
			return state, ErrRestoreSafety
		}
		prefix := ""
		if root != "" {
			prefix = RootPrefix + root + "/"
		}
		if info.Mode().IsRegular() {
			state.Kinds[root] = "file"
			fp, err := readRestoreRegularFile(path, info)
			if err != nil {
				return state, ErrRestoreSafety
			}
			name := prefix + filepath.Base(path)
			if _, ok := state.Entries[name]; ok {
				return state, ErrRestoreSafety
			}
			state.Entries[name] = fp
			continue
		}
		state.Kinds[root] = "directory"
		if prefix != "" {
			state.Entries[prefix] = restoreFingerprint{Directory: true}
		}
		err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return ErrRestoreSafety
			}
			if current == path {
				return nil
			}
			info, err := entry.Info()
			if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
				return ErrRestoreSafety
			}
			rel, err := filepath.Rel(path, current)
			if err != nil {
				return ErrRestoreSafety
			}
			name := prefix + filepath.ToSlash(rel)
			fp := restoreFingerprint{Directory: info.IsDir()}
			if fp.Directory {
				name += "/"
			} else {
				fp, err = readRestoreRegularFile(current, info)
				if err != nil {
					return ErrRestoreSafety
				}
			}
			if _, ok := state.Entries[name]; ok {
				return ErrRestoreSafety
			}
			state.Entries[name] = fp
			return nil
		})
		if err != nil {
			return state, ErrRestoreSafety
		}
	}
	return state, nil
}

func readRestoreRegularFile(path string, expected os.FileInfo) (restoreFingerprint, error) {
	var fp restoreFingerprint
	f, err := os.Open(path)
	if err != nil {
		return fp, ErrRestoreSafety
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(expected, info) {
		return fp, ErrRestoreSafety
	}
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil || n != info.Size() {
		return fp, ErrRestoreSafety
	}
	fp.Size = n
	copy(fp.Hash[:], sum.Sum(nil))
	return fp, nil
}

func sameRestoreCurrentState(a, b restoreCurrentState) bool {
	return maps.Equal(a.Entries, b.Entries) && maps.Equal(a.Kinds, b.Kinds)
}

// CreateVerifiedSafety reuses the restore capture gate for incoming sync.
// Create is intentionally best-effort for ordinary history; successful
// creation alone cannot authorize destruction of the current save.
func (m *Manager) CreateVerifiedSafety(gameID, comment string) error {
	game, err := m.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	roots, err := m.Store.GameRootPaths(gameID)
	if err != nil {
		return err
	}
	before, err := m.captureVerifiedCurrent(gameID, game.SavePath, roots, comment)
	if err != nil {
		return err
	}
	currentGame, err := m.Store.GetGame(gameID)
	if err != nil || currentGame.SavePath != game.SavePath {
		return ErrRestoreChanged
	}
	currentRoots, err := m.Store.GameRootPaths(gameID)
	if err != nil || !maps.Equal(roots, currentRoots) {
		return ErrRestoreChanged
	}
	after, err := readRestoreCurrentState(game.SavePath, roots)
	if err != nil || !sameRestoreCurrentState(before, after) {
		return ErrRestoreChanged
	}
	return nil
}

// Destructive operations share this capture gate. A valid ZIP alone cannot
// prove that every current file and empty directory made it into the archive.
func (m *Manager) captureVerifiedCurrent(gameID, primary string, extra map[string]string, comment string) (restoreCurrentState, error) {
	before, err := readRestoreCurrentState(primary, extra)
	if err != nil {
		return before, ErrRestoreSafety
	}
	if len(before.Entries) == 0 {
		return before, nil
	}
	safety, err := m.Create(gameID, comment, true)
	if err != nil {
		return before, ErrRestoreSafety
	}
	protected, err := restoreArchiveInventory(safety.ZipPath)
	if err != nil || !maps.Equal(before.Entries, protected) {
		return before, ErrRestoreSafety
	}
	return before, nil
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
