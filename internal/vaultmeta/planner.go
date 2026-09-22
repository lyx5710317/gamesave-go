package vaultmeta

import (
	"fmt"
	"regexp"
	"sort"
)

var contentHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// JoinMode describes only the vault-level decision. Applying one of these
// choices belongs to a separately confirmed provider workflow.
type JoinMode string

const (
	JoinCreateVault       JoinMode = "create-vault"
	JoinPublishLocalVault JoinMode = "publish-local-vault"
	JoinRemoteVault       JoinMode = "join-remote-vault"
	JoinAlreadyJoined     JoinMode = "already-joined"
	JoinVaultConflict     JoinMode = "vault-conflict"
)

// Relation describes what can be proved about one game's local and remote
// states. Timestamps intentionally do not participate.
type Relation string

const (
	RelationIdentical   Relation = "identical"
	RelationLocalOnly   Relation = "local-only"
	RelationRemoteOnly  Relation = "remote-only"
	RelationLocalAhead  Relation = "local-ahead"
	RelationRemoteAhead Relation = "remote-ahead"
	RelationConflict    Relation = "conflict"
)

// LibrarySummary is a portable summary of save content. Local paths and save
// bytes are intentionally absent so this object is safe to compare or display.
type LibrarySummary struct {
	Games []GameState `json:"games"`
}

// GameState describes one current game state. HeadSnapshotID and ancestors
// are populated only when immutable ancestry is known; legacy timestamp order
// must never be converted into ancestry.
type GameState struct {
	GameID              string   `json:"gameId"`
	Name                string   `json:"name"`
	ContentHash         string   `json:"contentHash"`
	HeadSnapshotID      string   `json:"headSnapshotId,omitempty"`
	AncestorSnapshotIDs []string `json:"ancestorSnapshotIds,omitempty"`
	LatestSnapshotID    string   `json:"latestSnapshotId,omitempty"`
	SnapshotCount       int      `json:"snapshotCount"`
	FileCount           int      `json:"fileCount"`
	TotalBytes          int64    `json:"totalBytes"`
	LatestAt            string   `json:"latestAt,omitempty"`
}

// Candidate is an untracked local save surfaced for explicit review. It has
// no filesystem path so it cannot accidentally leak into remote metadata.
type Candidate struct {
	ID          string `json:"id"`
	GroupID     string `json:"groupId,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	AppID       string `json:"appId,omitempty"`
	Role        string `json:"role,omitempty"`
	Measured    bool   `json:"measured"`
	FileCount   int    `json:"fileCount"`
	TotalBytes  int64  `json:"totalBytes"`
	LatestMtime int64  `json:"latestMtime"`
	Truncated   bool   `json:"truncated"`
}

// LocalScan combines tracked content with untracked discoveries. Complete is
// false when at least one candidate could not be measured; unknown never means
// empty.
type LocalScan struct {
	Library    LibrarySummary `json:"library"`
	Candidates []Candidate    `json:"candidates"`
	Complete   bool           `json:"complete"`
}

// GamePlan is one row in the non-mutating first-join preview.
type GamePlan struct {
	GameID             string     `json:"gameId"`
	Name               string     `json:"name"`
	Relation           Relation   `json:"relation"`
	Local              *GameState `json:"local,omitempty"`
	Remote             *GameState `json:"remote,omitempty"`
	RequiresResolution bool       `json:"requiresResolution"`
}

// JoinPlan is a preview, never an authorization to write. A provider workflow
// must collect the required confirmation and per-game conflict resolutions
// before it can call any mutation API.
type JoinPlan struct {
	Mode                 JoinMode    `json:"mode"`
	LocalVaultID         string      `json:"localVaultId,omitempty"`
	RemoteVaultID        string      `json:"remoteVaultId,omitempty"`
	Games                []GamePlan  `json:"games"`
	Candidates           []Candidate `json:"candidates"`
	LocalScanComplete    bool        `json:"localScanComplete"`
	RequiresLocalReview  bool        `json:"requiresLocalReview"`
	RequiresConfirmation bool        `json:"requiresConfirmation"`
	HasConflicts         bool        `json:"hasConflicts"`
}

// PlanFirstJoin validates both summaries and builds a deterministic preview.
// It never chooses a side by timestamp and exposes no apply operation.
func PlanFirstJoin(localVaultID string, remote *Metadata, local LocalScan, remoteLibrary LibrarySummary) (JoinPlan, error) {
	if localVaultID != "" {
		if err := validateVaultID(localVaultID); err != nil {
			return JoinPlan{}, fmt.Errorf("local vault id: %w", err)
		}
	}
	if remote != nil {
		if err := remote.Validate(); err != nil {
			return JoinPlan{}, fmt.Errorf("remote vault metadata: %w", err)
		}
	}
	if err := validateLibrary("local", local.Library); err != nil {
		return JoinPlan{}, err
	}
	if err := validateLibrary("remote", remoteLibrary); err != nil {
		return JoinPlan{}, err
	}
	if remote == nil && len(remoteLibrary.Games) > 0 {
		return JoinPlan{}, fmt.Errorf("remote library cannot be trusted without valid vault metadata")
	}
	if err := validateCandidates(local); err != nil {
		return JoinPlan{}, err
	}

	plan := JoinPlan{
		LocalVaultID:        localVaultID,
		Candidates:          append([]Candidate(nil), local.Candidates...),
		LocalScanComplete:   local.Complete,
		RequiresLocalReview: !local.Complete || len(local.Candidates) > 0,
	}
	sort.Slice(plan.Candidates, func(i, j int) bool {
		if plan.Candidates[i].GroupID != plan.Candidates[j].GroupID {
			return plan.Candidates[i].GroupID < plan.Candidates[j].GroupID
		}
		return plan.Candidates[i].ID < plan.Candidates[j].ID
	})

	switch {
	case remote == nil && localVaultID == "":
		plan.Mode = JoinCreateVault
	case remote == nil:
		plan.Mode = JoinPublishLocalVault
	case localVaultID == "":
		plan.Mode = JoinRemoteVault
		plan.RemoteVaultID = remote.VaultID
	case localVaultID == remote.VaultID:
		plan.Mode = JoinAlreadyJoined
		plan.RemoteVaultID = remote.VaultID
	default:
		plan.Mode = JoinVaultConflict
		plan.RemoteVaultID = remote.VaultID
		plan.RequiresConfirmation = true
		plan.HasConflicts = true
		return plan, nil
	}

	plan.Games = compareLibraries(local.Library, remoteLibrary)
	for _, game := range plan.Games {
		if game.Relation == RelationConflict {
			plan.HasConflicts = true
		}
	}
	plan.RequiresConfirmation = plan.Mode != JoinAlreadyJoined || plan.RequiresLocalReview || hasDifference(plan.Games)
	return plan, nil
}

func compareLibraries(local, remote LibrarySummary) []GamePlan {
	localByID := make(map[string]GameState, len(local.Games))
	remoteByID := make(map[string]GameState, len(remote.Games))
	ids := make(map[string]struct{}, len(local.Games)+len(remote.Games))
	for _, game := range local.Games {
		localByID[game.GameID] = game
		ids[game.GameID] = struct{}{}
	}
	for _, game := range remote.Games {
		remoteByID[game.GameID] = game
		ids[game.GameID] = struct{}{}
	}

	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)

	plans := make([]GamePlan, 0, len(ordered))
	for _, id := range ordered {
		localState, hasLocal := localByID[id]
		remoteState, hasRemote := remoteByID[id]
		plan := GamePlan{GameID: id}
		switch {
		case hasLocal && !hasRemote:
			plan.Name = localState.Name
			plan.Relation = RelationLocalOnly
			plan.Local = cloneGameState(localState)
		case !hasLocal && hasRemote:
			plan.Name = remoteState.Name
			plan.Relation = RelationRemoteOnly
			plan.Remote = cloneGameState(remoteState)
		default:
			plan.Name = localState.Name
			if plan.Name == "" {
				plan.Name = remoteState.Name
			}
			plan.Local = cloneGameState(localState)
			plan.Remote = cloneGameState(remoteState)
			plan.Relation = compareGame(localState, remoteState)
			plan.RequiresResolution = plan.Relation == RelationConflict
		}
		plans = append(plans, plan)
	}
	return plans
}

func compareGame(local, remote GameState) Relation {
	if local.ContentHash == remote.ContentHash {
		return RelationIdentical
	}
	localAhead := remote.HeadSnapshotID != "" && contains(local.AncestorSnapshotIDs, remote.HeadSnapshotID)
	remoteAhead := local.HeadSnapshotID != "" && contains(remote.AncestorSnapshotIDs, local.HeadSnapshotID)
	if localAhead == remoteAhead {
		// Neither direction is proven, or the supplied histories form a cycle.
		// Both are conflicts; picking the first match would silently choose a
		// side from contradictory metadata.
		return RelationConflict
	}
	if localAhead {
		return RelationLocalAhead
	}
	if remoteAhead {
		return RelationRemoteAhead
	}
	return RelationConflict
}

func validateLibrary(side string, library LibrarySummary) error {
	seen := make(map[string]struct{}, len(library.Games))
	for i, game := range library.Games {
		if game.GameID == "" {
			return fmt.Errorf("%s library game %d has no game id", side, i)
		}
		if _, exists := seen[game.GameID]; exists {
			return fmt.Errorf("%s library contains duplicate game id %q", side, game.GameID)
		}
		seen[game.GameID] = struct{}{}
		if game.Name == "" {
			return fmt.Errorf("%s library game %q has no name", side, game.GameID)
		}
		if !contentHashPattern.MatchString(game.ContentHash) {
			return fmt.Errorf("%s library game %q has an invalid content hash", side, game.GameID)
		}
		if game.SnapshotCount < 0 || game.FileCount < 0 || game.TotalBytes < 0 {
			return fmt.Errorf("%s library game %q has negative counts", side, game.GameID)
		}
		if game.HeadSnapshotID == "" && len(game.AncestorSnapshotIDs) > 0 {
			return fmt.Errorf("%s library game %q has ancestors but no immutable head", side, game.GameID)
		}
		ancestors := make(map[string]struct{}, len(game.AncestorSnapshotIDs))
		for _, ancestor := range game.AncestorSnapshotIDs {
			if ancestor == "" || ancestor == game.HeadSnapshotID {
				return fmt.Errorf("%s library game %q has invalid snapshot ancestry", side, game.GameID)
			}
			if _, exists := ancestors[ancestor]; exists {
				return fmt.Errorf("%s library game %q has duplicate snapshot ancestry", side, game.GameID)
			}
			ancestors[ancestor] = struct{}{}
		}
	}
	return nil
}

func validateCandidates(local LocalScan) error {
	seen := make(map[string]struct{}, len(local.Candidates))
	for i, candidate := range local.Candidates {
		if candidate.ID == "" || candidate.Name == "" {
			return fmt.Errorf("local candidate %d must have an id and name", i)
		}
		if _, exists := seen[candidate.ID]; exists {
			return fmt.Errorf("local scan contains duplicate candidate id %q", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		if candidate.FileCount < 0 || candidate.TotalBytes < 0 || candidate.LatestMtime < 0 {
			return fmt.Errorf("local candidate %q has negative measurements", candidate.ID)
		}
		if local.Complete && !candidate.Measured {
			return fmt.Errorf("local scan is marked complete but candidate %q is unmeasured", candidate.ID)
		}
	}
	return nil
}

func cloneGameState(game GameState) *GameState {
	game.AncestorSnapshotIDs = append([]string(nil), game.AncestorSnapshotIDs...)
	return &game
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasDifference(games []GamePlan) bool {
	for _, game := range games {
		if game.Relation != RelationIdentical {
			return true
		}
	}
	return false
}
