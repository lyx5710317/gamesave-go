package vaultmeta

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// ScanLocal builds the provider-independent first-join view from the current
// filesystem, store, and an already detected/measured candidate list. It does
// not create snapshots or write to the store.
func ScanLocal(s *store.Store, discovered []presets.DiscoveredSave) (LocalScan, error) {
	library, err := scanTrackedLibrary(s)
	if err != nil {
		return LocalScan{}, err
	}
	candidates, complete := summarizeCandidates(discovered)
	return LocalScan{Library: library, Candidates: candidates, Complete: complete}, nil
}

func scanTrackedLibrary(s *store.Store) (LibrarySummary, error) {
	games, err := s.ListGames()
	if err != nil {
		return LibrarySummary{}, err
	}

	summary := LibrarySummary{Games: make([]GameState, 0, len(games))}
	for _, game := range games {
		extra, err := s.GameRootPaths(game.ID)
		if err != nil {
			return LibrarySummary{}, fmt.Errorf("scan local game %s roots: %w", game.ID, err)
		}
		manifest, failures, err := delta.BuildMultiManifest(game.SavePath, extra)
		if err != nil {
			return LibrarySummary{}, fmt.Errorf("scan local game %s: %w", game.ID, err)
		}
		if len(failures) > 0 {
			names := make([]string, 0, len(failures))
			for name := range failures {
				names = append(names, name)
			}
			sort.Strings(names)
			parts := make([]string, 0, len(names))
			for _, name := range names {
				parts = append(parts, fmt.Sprintf("%s: %v", name, failures[name]))
			}
			return LibrarySummary{}, fmt.Errorf("scan local game %s has unreadable save locations: %s", game.ID, strings.Join(parts, "; "))
		}

		state := GameState{
			GameID:      game.ID,
			Name:        game.Name,
			ContentHash: manifest.ContentHash(),
		}
		for _, root := range manifest.RootNames() {
			for _, entry := range manifest.Root(root).Files {
				state.FileCount++
				state.TotalBytes += entry.Size
			}
		}
		if manifest.LatestMtime > 0 {
			state.LatestAt = time.UnixMilli(int64(manifest.LatestMtime)).UTC().Format(time.RFC3339Nano)
		}

		branches, err := s.ListBranches(game.ID)
		if err != nil {
			return LibrarySummary{}, fmt.Errorf("scan local game %s branches: %w", game.ID, err)
		}
		for _, branch := range branches {
			snapshots, err := s.ListSnapshots(game.ID, branch)
			if err != nil {
				return LibrarySummary{}, fmt.Errorf("scan local game %s snapshots: %w", game.ID, err)
			}
			state.SnapshotCount += len(snapshots)
			if branch == game.ActiveBranch && len(snapshots) > 0 {
				state.LatestSnapshotID = snapshots[0].ID
			}
		}
		// Existing rows have neither an immutable head nor parent links. Keep
		// HeadSnapshotID/AncestorSnapshotIDs empty rather than inventing
		// ancestry from timestamps.
		summary.Games = append(summary.Games, state)
	}
	sort.Slice(summary.Games, func(i, j int) bool { return summary.Games[i].GameID < summary.Games[j].GameID })
	return summary, nil
}

func summarizeCandidates(discovered []presets.DiscoveredSave) ([]Candidate, bool) {
	candidates := make([]Candidate, 0, len(discovered))
	complete := true
	for _, save := range discovered {
		if !save.Measured {
			complete = false
		}
		candidates = append(candidates, Candidate{
			ID:          save.ID,
			GroupID:     save.GroupID,
			Name:        save.Name,
			Type:        save.Type,
			AppID:       save.AppID,
			Role:        save.Role,
			Measured:    save.Measured,
			FileCount:   save.FileCount,
			TotalBytes:  save.TotalBytes,
			LatestMtime: save.LatestMtime,
			Truncated:   save.Truncated,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].GroupID != candidates[j].GroupID {
			return candidates[i].GroupID < candidates[j].GroupID
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates, complete
}
