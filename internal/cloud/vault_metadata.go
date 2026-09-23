package cloud

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/opensave/opensave/internal/vaultmeta"
)

var (
	// ErrVaultMetadataUnsupported prevents a provider without a strong
	// conditional-write primitive from silently using last-writer-wins.
	ErrVaultMetadataUnsupported = errors.New("provider does not support conditional vault metadata writes")
	// ErrVaultMetadataNotFound means no vault.json exists at the provider path.
	ErrVaultMetadataNotFound = errors.New("remote vault metadata not found")
	// ErrVaultRevisionConflict means another writer won the compare-and-swap.
	// Callers must reread and merge; they must never retry the stale body.
	ErrVaultRevisionConflict  = errors.New("remote vault metadata changed concurrently")
	ErrVaultInvalidTransition = errors.New("invalid vault metadata transition")
)

const maxVaultMetadataBytes = 1 << 20

// VaultMetadataProvider is an optional capability of a snapshot Provider.
// versionToken is an opaque provider revision/ETag for the exact account and
// vault.json object, not vault.json's numeric revision. A token from another
// account or object must never match. ReplaceVaultMetadata must compare it
// atomically and return ErrVaultRevisionConflict without writing if stale.
type VaultMetadataProvider interface {
	ReadVaultMetadata(ctx context.Context) (data []byte, versionToken string, err error)
	ReplaceVaultMetadata(ctx context.Context, expectedVersion string, data []byte) (newVersion string, err error)
}

// VaultState is one validated remote read and its provider precondition.
// A stale state cannot authorize a later metadata overwrite.
type VaultState struct {
	Metadata     vaultmeta.Metadata
	VersionToken string
	readDigest   [sha256.Size]byte
	tokenDigest  [sha256.Size]byte
	providerName string
}

// ReadVaultMetadata validates remote metadata before exposing it to join
// planning. Providers lacking conditional writes are deliberately rejected,
// even for reads, because they cannot safely complete a subsequent join.
func (s *Service) ReadVaultMetadata(ctx context.Context) (VaultState, error) {
	if err := ctx.Err(); err != nil {
		return VaultState{}, err
	}
	provider, providerName, err := s.vaultMetadataProvider()
	if err != nil {
		return VaultState{}, err
	}
	data, version, err := provider.ReadVaultMetadata(ctx)
	if err != nil {
		return VaultState{}, err
	}
	if version == "" || strings.TrimSpace(version) != version {
		return VaultState{}, errors.New("provider returned no usable vault version token")
	}
	if len(data) == 0 || len(data) > maxVaultMetadataBytes {
		return VaultState{}, errors.New("remote vault metadata is empty or too large")
	}
	metadata, err := vaultmeta.Parse(data)
	if err != nil {
		return VaultState{}, err
	}
	canonical, err := vaultmeta.Marshal(metadata)
	if err != nil {
		return VaultState{}, err
	}
	return VaultState{
		Metadata: metadata, VersionToken: version,
		readDigest: sha256.Sum256(canonical), tokenDigest: sha256.Sum256([]byte(version)),
		providerName: providerName,
	}, nil
}

// ReplaceVaultMetadata checks the immutable vault identity, monotonic
// revision, and existing device registrations before one provider CAS. It
// does not create vaults or register keys: callers must complete the separate
// wrapped-key safety prerequisites before proposing a new device.
func (s *Service) ReplaceVaultMetadata(ctx context.Context, observed VaultState, next vaultmeta.Metadata) (VaultState, error) {
	if err := ctx.Err(); err != nil {
		return VaultState{}, err
	}
	provider, providerName, err := s.vaultMetadataProvider()
	if err != nil {
		return VaultState{}, err
	}
	if observed.providerName != providerName {
		return VaultState{}, fmt.Errorf("%w: cloud provider changed since read", ErrVaultInvalidTransition)
	}
	if observed.VersionToken == "" || strings.TrimSpace(observed.VersionToken) != observed.VersionToken {
		return VaultState{}, fmt.Errorf("%w: missing provider precondition", ErrVaultInvalidTransition)
	}
	if observed.tokenDigest != sha256.Sum256([]byte(observed.VersionToken)) {
		return VaultState{}, fmt.Errorf("%w: provider precondition changed after read", ErrVaultInvalidTransition)
	}
	if err := observed.Metadata.Validate(); err != nil {
		return VaultState{}, fmt.Errorf("%w: old metadata: %v", ErrVaultInvalidTransition, err)
	}
	oldCanonical, err := vaultmeta.Marshal(observed.Metadata)
	if err != nil || observed.readDigest != sha256.Sum256(oldCanonical) {
		return VaultState{}, fmt.Errorf("%w: observed metadata changed after read", ErrVaultInvalidTransition)
	}
	if err := next.Validate(); err != nil {
		return VaultState{}, fmt.Errorf("%w: next metadata: %v", ErrVaultInvalidTransition, err)
	}
	if err := validateVaultTransition(observed.Metadata, next); err != nil {
		return VaultState{}, err
	}
	data, err := vaultmeta.Marshal(next)
	if err != nil {
		return VaultState{}, err
	}
	if len(data) > maxVaultMetadataBytes {
		return VaultState{}, fmt.Errorf("%w: metadata exceeds size limit", ErrVaultInvalidTransition)
	}
	newVersion, err := provider.ReplaceVaultMetadata(ctx, observed.VersionToken, data)
	if err != nil {
		return VaultState{}, err
	}
	if newVersion == "" || strings.TrimSpace(newVersion) != newVersion {
		return VaultState{}, errors.New("provider wrote vault metadata but returned no version token; reread before continuing")
	}
	return VaultState{
		Metadata: next, VersionToken: newVersion,
		readDigest: sha256.Sum256(data), tokenDigest: sha256.Sum256([]byte(newVersion)),
		providerName: providerName,
	}, nil
}

func (s *Service) vaultMetadataProvider() (VaultMetadataProvider, string, error) {
	name, provider, err := s.selectedProviderNamed()
	if err != nil {
		return nil, "", err
	}
	capability, ok := provider.(VaultMetadataProvider)
	if !ok {
		return nil, "", ErrVaultMetadataUnsupported
	}
	return capability, name, nil
}

func validateVaultTransition(old, next vaultmeta.Metadata) error {
	if old.VaultID != next.VaultID || old.CreatedAt != next.CreatedAt || old.SchemaVersion != next.SchemaVersion {
		return fmt.Errorf("%w: vault identity or creation metadata changed", ErrVaultInvalidTransition)
	}
	if old.Revision == ^uint64(0) || next.Revision != old.Revision+1 {
		return fmt.Errorf("%w: revision must increase by exactly one", ErrVaultInvalidTransition)
	}
	newDevices := make(map[string]vaultmeta.Device, len(next.Devices))
	for _, device := range next.Devices {
		newDevices[device.DeviceID] = device
	}
	for _, previous := range old.Devices {
		current, ok := newDevices[previous.DeviceID]
		if !ok {
			return fmt.Errorf("%w: existing device registration was removed", ErrVaultInvalidTransition)
		}
		if current.NodeID != previous.NodeID || current.IdentityPublicKey != previous.IdentityPublicKey ||
			current.RegisteredAt != previous.RegisteredAt ||
			(previous.RevokedAt != nil && (current.RevokedAt == nil || *current.RevokedAt != *previous.RevokedAt)) {
			return fmt.Errorf("%w: existing device identity or revocation changed", ErrVaultInvalidTransition)
		}
	}
	return nil
}
