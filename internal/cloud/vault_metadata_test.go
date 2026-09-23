package cloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/vaultmeta"
)

type memoryVaultProvider struct {
	recordingProvider
	mu      sync.Mutex
	data    []byte
	version int
	writes  int
}

func (p *memoryVaultProvider) ReadVaultMetadata(ctx context.Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.data == nil {
		return nil, "", ErrVaultMetadataNotFound
	}
	return append([]byte(nil), p.data...), fmt.Sprintf("etag-%d", p.version), nil
}

func (p *memoryVaultProvider) ReplaceVaultMetadata(ctx context.Context, expected string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if expected != fmt.Sprintf("etag-%d", p.version) {
		return "", ErrVaultRevisionConflict
	}
	p.data = append([]byte(nil), data...)
	p.version++
	p.writes++
	return fmt.Sprintf("etag-%d", p.version), nil
}

func testVaultDevice(t *testing.T, name string) vaultmeta.Device {
	t.Helper()
	id := uuid.NewString()
	identity, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return vaultmeta.Device{
		DeviceID: id, NodeID: "node_" + strings.ReplaceAll(id, "-", ""),
		Name: name, IdentityPublicKey: e2ee.EncodeKey(identity.Public),
		RegisteredAt: "2026-09-22T10:00:00Z",
	}
}

func testVaultMetadata(t *testing.T) vaultmeta.Metadata {
	t.Helper()
	return vaultmeta.Metadata{
		SchemaVersion: vaultmeta.SchemaVersion,
		VaultID:       uuid.NewString(),
		Revision:      1,
		CreatedAt:     "2026-09-22T10:00:00Z",
		UpdatedAt:     "2026-09-22T10:00:00Z",
		Devices:       []vaultmeta.Device{testVaultDevice(t, "Gaming PC")},
	}
}

func testVaultService(t *testing.T, provider *memoryVaultProvider) *Service {
	t.Helper()
	svc, db := newTestService(t)
	if err := svc.RegisterProvider("test_vault", provider); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "test_vault"
	})
	return svc
}

func TestVaultMetadataConcurrentWritersMustReread(t *testing.T) {
	metadata := testVaultMetadata(t)
	raw, err := vaultmeta.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	provider := &memoryVaultProvider{data: raw, version: 1}
	svc := testVaultService(t, provider)
	first, err := svc.ReadVaultMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stale, err := svc.ReadVaultMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionToken != "etag-1" || first.Metadata.VaultID != metadata.VaultID {
		t.Fatalf("ReadVaultMetadata = %+v", first)
	}
	next := first.Metadata
	next.Revision++
	next.UpdatedAt = "2026-09-22T10:01:00Z"
	next.Devices = append(append([]vaultmeta.Device(nil), next.Devices...), testVaultDevice(t, "Laptop"))
	updated, err := svc.ReplaceVaultMetadata(context.Background(), first, next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.VersionToken != "etag-2" || updated.Metadata.Revision != 2 {
		t.Fatalf("ReplaceVaultMetadata = %+v", updated)
	}
	other := stale.Metadata
	other.Revision++
	other.UpdatedAt = "2026-09-22T10:02:00Z"
	other.Devices = append(append([]vaultmeta.Device(nil), other.Devices...), testVaultDevice(t, "Steam Deck"))
	if _, err := svc.ReplaceVaultMetadata(context.Background(), stale, other); !errors.Is(err, ErrVaultRevisionConflict) {
		t.Fatalf("stale replacement = %v, want revision conflict", err)
	}
	current, err := svc.ReadVaultMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	hasLaptop, hasSteamDeck := false, false
	for _, device := range current.Metadata.Devices {
		hasLaptop = hasLaptop || device.Name == "Laptop"
		hasSteamDeck = hasSteamDeck || device.Name == "Steam Deck"
	}
	if current.Metadata.Revision != 2 || len(current.Metadata.Devices) != 2 || !hasLaptop || hasSteamDeck || provider.writes != 1 {
		t.Fatalf("stale write changed remote vault: %+v, writes=%d", current.Metadata, provider.writes)
	}
}

func TestVaultMetadataRejectsUnsafeTransitionsBeforeProviderWrite(t *testing.T) {
	metadata := testVaultMetadata(t)
	raw, err := vaultmeta.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	provider := &memoryVaultProvider{data: raw, version: 1}
	svc := testVaultService(t, provider)
	observed, err := svc.ReadVaultMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tampered := observed
	tampered.Metadata.Devices = append([]vaultmeta.Device(nil), tampered.Metadata.Devices...)
	tampered.Metadata.Devices[0].Name = "renamed after read"
	tamperedNext := tampered.Metadata
	tamperedNext.Revision = 2
	if _, err := svc.ReplaceVaultMetadata(context.Background(), tampered, tamperedNext); !errors.Is(err, ErrVaultInvalidTransition) {
		t.Fatalf("mutated observed state = %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*vaultmeta.Metadata)
	}{
		{"same revision", func(m *vaultmeta.Metadata) {}},
		{"skipped revision", func(m *vaultmeta.Metadata) { m.Revision = 3 }},
		{"different vault", func(m *vaultmeta.Metadata) { m.Revision = 2; m.VaultID = uuid.NewString() }},
		{"changed creation", func(m *vaultmeta.Metadata) { m.Revision = 2; m.CreatedAt = "2026-09-22T09:00:00Z" }},
		{"removed device", func(m *vaultmeta.Metadata) {
			m.Revision = 2
			m.Devices = []vaultmeta.Device{testVaultDevice(t, "Other")}
		}},
		{"changed identity key", func(m *vaultmeta.Metadata) {
			m.Revision = 2
			m.Devices = append([]vaultmeta.Device(nil), m.Devices...)
			m.Devices[0].IdentityPublicKey = testVaultDevice(t, "Other").IdentityPublicKey
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := observed.Metadata
			tc.mutate(&next)
			if _, err := svc.ReplaceVaultMetadata(context.Background(), observed, next); !errors.Is(err, ErrVaultInvalidTransition) {
				t.Fatalf("unsafe transition = %v", err)
			}
		})
	}
	if provider.writes != 0 {
		t.Fatalf("unsafe transitions caused %d remote writes", provider.writes)
	}
}

func TestVaultMetadataReadRejectsUnsupportedAndMalformedSources(t *testing.T) {
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "local"
	})
	if _, err := svc.ReadVaultMetadata(context.Background()); !errors.Is(err, ErrVaultMetadataUnsupported) {
		t.Fatalf("legacy provider = %v, want unsupported", err)
	}

	provider := &memoryVaultProvider{version: 1}
	svc = testVaultService(t, provider)
	if _, err := svc.ReadVaultMetadata(context.Background()); !errors.Is(err, ErrVaultMetadataNotFound) {
		t.Fatalf("absent metadata = %v, want not found", err)
	}
	provider.data = []byte("not-json")
	if _, err := svc.ReadVaultMetadata(context.Background()); !errors.Is(err, vaultmeta.ErrInvalidMetadata) {
		t.Fatalf("malformed metadata = %v", err)
	}
	provider.data = []byte(`{"schemaVersion":2}`)
	if _, err := svc.ReadVaultMetadata(context.Background()); !errors.Is(err, vaultmeta.ErrUnsupportedSchema) {
		t.Fatalf("future schema = %v, want upgrade-required error", err)
	}
	provider.data = []byte(strings.Repeat("x", maxVaultMetadataBytes+1))
	if _, err := svc.ReadVaultMetadata(context.Background()); err == nil {
		t.Fatal("oversized metadata accepted")
	}
	if provider.writes != 0 {
		t.Fatalf("read-only discovery caused %d writes", provider.writes)
	}
}

func TestVaultMetadataCannotUndoRevocation(t *testing.T) {
	metadata := testVaultMetadata(t)
	revokedAt := "2026-09-22T11:00:00Z"
	metadata.Devices[0].RevokedAt = &revokedAt
	metadata.UpdatedAt = revokedAt
	raw, err := vaultmeta.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	provider := &memoryVaultProvider{data: raw, version: 1}
	svc := testVaultService(t, provider)
	observed, err := svc.ReadVaultMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next := observed.Metadata
	next.Revision++
	next.Devices = append([]vaultmeta.Device(nil), next.Devices...)
	next.Devices[0].RevokedAt = nil
	if _, err := svc.ReplaceVaultMetadata(context.Background(), observed, next); !errors.Is(err, ErrVaultInvalidTransition) {
		t.Fatalf("revocation rollback = %v", err)
	}
	if provider.writes != 0 {
		t.Fatal("revocation rollback reached provider")
	}
}

func TestVaultMetadataCancelledContextDoesNotReadOrWrite(t *testing.T) {
	provider := &memoryVaultProvider{version: 1}
	svc := testVaultService(t, provider)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.ReadVaultMetadata(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}
	if _, err := svc.ReplaceVaultMetadata(ctx, VaultState{}, vaultmeta.Metadata{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write = %v", err)
	}
	if provider.writes != 0 {
		t.Fatal("cancelled context wrote metadata")
	}
}

func TestVaultMetadataReadCannotAuthorizeWriteToAnotherProvider(t *testing.T) {
	metadata := testVaultMetadata(t)
	raw, err := vaultmeta.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	svc, db := newTestService(t)
	first := &memoryVaultProvider{data: raw, version: 1}
	second := &memoryVaultProvider{data: raw, version: 1}
	if err := svc.RegisterProvider("vault_first", first); err != nil {
		t.Fatal(err)
	}
	if err := svc.RegisterProvider("vault_second", second); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "vault_first"
	})
	observed, err := svc.ReadVaultMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) { c.Provider = "vault_second" })
	next := observed.Metadata
	next.Revision++
	if _, err := svc.ReplaceVaultMetadata(context.Background(), observed, next); !errors.Is(err, ErrVaultInvalidTransition) {
		t.Fatalf("cross-provider write = %v", err)
	}
	if first.writes != 0 || second.writes != 0 {
		t.Fatalf("cross-provider write reached an adapter: first=%d, second=%d", first.writes, second.writes)
	}
}
