package vaultmeta

import (
	"strings"
	"testing"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hashC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestPlanFirstJoinModeMatrix(t *testing.T) {
	remote := validMetadata(t)
	local := LocalScan{Library: library(game("same", hashA)), Complete: true}
	remoteLibrary := library(game("same", hashA))

	tests := []struct {
		name         string
		localVaultID string
		remote       *Metadata
		want         JoinMode
	}{
		{name: "create", want: JoinCreateVault},
		{name: "publish local", localVaultID: testVaultID, want: JoinPublishLocalVault},
		{name: "join remote", remote: &remote, want: JoinRemoteVault},
		{name: "already joined", localVaultID: testVaultID, remote: &remote, want: JoinAlreadyJoined},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remoteSummary := remoteLibrary
			if tt.remote == nil {
				remoteSummary = LibrarySummary{}
			}
			plan, err := PlanFirstJoin(tt.localVaultID, tt.remote, local, remoteSummary)
			if err != nil {
				t.Fatalf("PlanFirstJoin() error = %v", err)
			}
			if plan.Mode != tt.want {
				t.Fatalf("Mode = %q, want %q", plan.Mode, tt.want)
			}
			if tt.want == JoinAlreadyJoined && plan.RequiresConfirmation {
				t.Fatal("identical already-joined libraries unexpectedly require confirmation")
			}
		})
	}
}

func TestPlanFirstJoinStopsOnDifferentVaults(t *testing.T) {
	remote := validMetadata(t)
	plan, err := PlanFirstJoin(
		"34675249-7c0a-4a4c-a749-d290e79ae1d7",
		&remote,
		LocalScan{Library: library(game("local", hashA)), Complete: true},
		library(game("remote", hashB)),
	)
	if err != nil {
		t.Fatalf("PlanFirstJoin() error = %v", err)
	}
	if plan.Mode != JoinVaultConflict || !plan.HasConflicts || !plan.RequiresConfirmation {
		t.Fatalf("PlanFirstJoin() = %#v", plan)
	}
	if len(plan.Games) != 0 {
		t.Fatalf("vault conflict should not propose per-game actions: %#v", plan.Games)
	}
}

func TestPlanFirstJoinClassifiesGamesByHashAndProvenAncestry(t *testing.T) {
	remote := validMetadata(t)
	local := LocalScan{
		Complete: true,
		Library: library(
			game("identical", hashA),
			game("local-only", hashA),
			withHistory(game("local-ahead", hashB), "local-head", "common-head"),
			withHistory(game("remote-ahead", hashA), "common-head"),
			game("conflict", hashB),
		),
	}
	remoteLibrary := library(
		game("identical", hashA),
		game("remote-only", hashC),
		withHistory(game("local-ahead", hashA), "common-head"),
		withHistory(game("remote-ahead", hashB), "remote-head", "common-head"),
		game("conflict", hashC),
	)

	plan, err := PlanFirstJoin("", &remote, local, remoteLibrary)
	if err != nil {
		t.Fatalf("PlanFirstJoin() error = %v", err)
	}
	want := map[string]Relation{
		"identical":    RelationIdentical,
		"local-ahead":  RelationLocalAhead,
		"local-only":   RelationLocalOnly,
		"remote-ahead": RelationRemoteAhead,
		"remote-only":  RelationRemoteOnly,
		"conflict":     RelationConflict,
	}
	if len(plan.Games) != len(want) {
		t.Fatalf("len(Games) = %d, want %d", len(plan.Games), len(want))
	}
	previous := ""
	for _, row := range plan.Games {
		if row.GameID < previous {
			t.Fatalf("games not sorted: %#v", plan.Games)
		}
		previous = row.GameID
		if row.Relation != want[row.GameID] {
			t.Errorf("game %q relation = %q, want %q", row.GameID, row.Relation, want[row.GameID])
		}
		if row.RequiresResolution != (row.Relation == RelationConflict) {
			t.Errorf("game %q RequiresResolution = %v", row.GameID, row.RequiresResolution)
		}
	}
	if !plan.HasConflicts || !plan.RequiresConfirmation {
		t.Fatalf("plan safety flags = %#v", plan)
	}
}

func TestPlanFirstJoinNeverUsesTimestampAsAncestry(t *testing.T) {
	remote := validMetadata(t)
	localGame := game("timestamp-trap", hashA)
	localGame.LatestAt = "2030-01-01T00:00:00Z"
	remoteGame := game("timestamp-trap", hashB)
	remoteGame.LatestAt = "2020-01-01T00:00:00Z"

	plan, err := PlanFirstJoin("", &remote, LocalScan{
		Library:  library(localGame),
		Complete: true,
	}, library(remoteGame))
	if err != nil {
		t.Fatalf("PlanFirstJoin() error = %v", err)
	}
	if got := plan.Games[0].Relation; got != RelationConflict {
		t.Fatalf("Relation = %q, want conflict despite newer local timestamp", got)
	}
}

func TestPlanFirstJoinRejectsContradictoryAncestry(t *testing.T) {
	remote := validMetadata(t)
	localGame := withHistory(game("cycle", hashA), "local-head", "remote-head")
	remoteGame := withHistory(game("cycle", hashB), "remote-head", "local-head")

	plan, err := PlanFirstJoin("", &remote, LocalScan{
		Library:  library(localGame),
		Complete: true,
	}, library(remoteGame))
	if err != nil {
		t.Fatalf("PlanFirstJoin() error = %v", err)
	}
	if got := plan.Games[0].Relation; got != RelationConflict {
		t.Fatalf("Relation = %q, want conflict for cyclic ancestry", got)
	}
}

func TestPlanFirstJoinSurfacesUnmeasuredCandidates(t *testing.T) {
	remote := validMetadata(t)
	local := LocalScan{
		Library:  LibrarySummary{},
		Complete: false,
		Candidates: []Candidate{
			{ID: "z", Name: "Unknown Save"},
			{ID: "a", Name: "Measured Save", Measured: true},
		},
	}
	plan, err := PlanFirstJoin("", &remote, local, LibrarySummary{})
	if err != nil {
		t.Fatalf("PlanFirstJoin() error = %v", err)
	}
	if !plan.RequiresLocalReview || !plan.RequiresConfirmation || plan.LocalScanComplete {
		t.Fatalf("candidate safety flags = %#v", plan)
	}
	if plan.Candidates[0].ID != "a" || plan.Candidates[1].ID != "z" {
		t.Fatalf("candidates not deterministically sorted: %#v", plan.Candidates)
	}
}

func TestPlanFirstJoinRejectsInvalidLibrary(t *testing.T) {
	remote := validMetadata(t)
	_, err := PlanFirstJoin("", &remote, LocalScan{
		Library:  library(game("broken", "not-a-hash")),
		Complete: true,
	}, LibrarySummary{})
	if err == nil || !strings.Contains(err.Error(), "content hash") {
		t.Fatalf("PlanFirstJoin() error = %v, want content hash validation", err)
	}
}

func TestPlanFirstJoinRejectsRemoteLibraryWithoutMetadata(t *testing.T) {
	_, err := PlanFirstJoin("", nil, LocalScan{Complete: true}, library(game("untrusted", hashA)))
	if err == nil || !strings.Contains(err.Error(), "without valid vault metadata") {
		t.Fatalf("PlanFirstJoin() error = %v, want missing-metadata validation", err)
	}
}

func TestPlanFirstJoinRejectsFalselyCompleteCandidateScan(t *testing.T) {
	remote := validMetadata(t)
	_, err := PlanFirstJoin("", &remote, LocalScan{
		Complete:   true,
		Candidates: []Candidate{{ID: "unknown", Name: "Unknown"}},
	}, LibrarySummary{})
	if err == nil || !strings.Contains(err.Error(), "marked complete") {
		t.Fatalf("PlanFirstJoin() error = %v, want candidate completeness validation", err)
	}
}

func game(id, hash string) GameState {
	return GameState{GameID: id, Name: id, ContentHash: hash}
}

func withHistory(state GameState, head string, ancestors ...string) GameState {
	state.HeadSnapshotID = head
	state.AncestorSnapshotIDs = append([]string(nil), ancestors...)
	return state
}

func library(games ...GameState) LibrarySummary {
	return LibrarySummary{Games: games}
}
