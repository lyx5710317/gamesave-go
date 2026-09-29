package vaultmeta

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/e2ee"
)

const (
	testVaultID   = "80cc57d6-03c4-4db4-bf2d-8c93a5ca4efa"
	testDeviceID1 = "7e2ae42d-4672-4e48-b840-21f72afbe3dc"
	testNodeID1   = "node_7e2ae42d46724e48b84021f72afbe3dc"
	testDeviceID2 = "8b0b5651-9932-42d8-a2e8-935c8c49e033"
	testNodeID2   = "node_8b0b5651993242d8a2e8935c8c49e033"
)

func TestParseAndMarshalMetadataDeterministically(t *testing.T) {
	metadata := validMetadata(t)
	metadata.Devices = []Device{
		validDevice(t, testDeviceID2, testNodeID2, "Steam Deck"),
		validDevice(t, testDeviceID1, testNodeID1, "Gaming PC"),
	}

	data, err := Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if bytes.Index(data, []byte(testDeviceID1)) > bytes.Index(data, []byte(testDeviceID2)) {
		t.Fatalf("Marshal() did not sort devices by id:\n%s", data)
	}
	if metadata.Devices[0].DeviceID != testDeviceID2 {
		t.Fatal("Marshal() mutated the caller's device order")
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.VaultID != testVaultID || len(parsed.Devices) != 2 {
		t.Fatalf("Parse() = %#v", parsed)
	}
}

func TestParseAcceptsLegacyVaultIDWithoutReformatting(t *testing.T) {
	metadata := validMetadata(t)
	metadata.VaultID = "80cc57d603c44db4bf2d8c93a5ca4efa"

	data, err := Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.VaultID != metadata.VaultID {
		t.Fatalf("VaultID = %q, want exact legacy value %q", parsed.VaultID, metadata.VaultID)
	}
}

func TestParseRejectsUnsupportedSchema(t *testing.T) {
	metadata := validMetadata(t)
	metadata.SchemaVersion = 2
	data := mustJSONWithoutValidation(t, metadata)

	_, err := Parse(data)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("Parse() error = %v, want ErrUnsupportedSchema", err)
	}
}

func TestMetadataValidationRejectsUnsafeIdentityChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Metadata)
		want   string
	}{
		{
			name: "non v4 vault id",
			mutate: func(metadata *Metadata) {
				metadata.VaultID = "80cc57d6-03c3-3db4-bf2d-8c93a5ca4efa"
			},
			want: "vaultId",
		},
		{
			name: "duplicate device",
			mutate: func(metadata *Metadata) {
				metadata.Devices = append(metadata.Devices, metadata.Devices[0])
			},
			want: "duplicate deviceId",
		},
		{
			name: "node and device mismatch",
			mutate: func(metadata *Metadata) {
				metadata.Devices[0].NodeID = testNodeID2
			},
			want: "same installation",
		},
		{
			name: "bad public key",
			mutate: func(metadata *Metadata) {
				metadata.Devices[0].IdentityPublicKey = "not-a-key"
			},
			want: "X25519",
		},
		{
			name: "non utc timestamp",
			mutate: func(metadata *Metadata) {
				metadata.UpdatedAt = "2026-09-22T18:15:00+08:00"
			},
			want: "UTC",
		},
		{
			name: "revocation before registration",
			mutate: func(metadata *Metadata) {
				revoked := "2026-09-21T10:00:00Z"
				metadata.Devices[0].RevokedAt = &revoked
			},
			want: "precedes registeredAt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := validMetadata(t)
			tt.mutate(&metadata)
			err := metadata.Validate()
			if !errors.Is(err, ErrInvalidMetadata) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want ErrInvalidMetadata containing %q", err, tt.want)
			}
		})
	}
}

func TestParseRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	data, err := Marshal(validMetadata(t))
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(data, []byte(`"revision": 1`), []byte(`"revision": 1, "secretKey": "forbidden"`), 1)
	if _, err := Parse(unknown); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("Parse(unknown) error = %v, want ErrInvalidMetadata", err)
	}
	trailing := append(append([]byte(nil), data...), []byte(`{}`)...)
	if _, err := Parse(trailing); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("Parse(trailing) error = %v, want ErrInvalidMetadata", err)
	}
}

func TestDeviceIDFromNodeID(t *testing.T) {
	got, err := DeviceIDFromNodeID(testNodeID1)
	if err != nil {
		t.Fatalf("DeviceIDFromNodeID() error = %v", err)
	}
	if got != testDeviceID1 {
		t.Fatalf("DeviceIDFromNodeID() = %q, want %q", got, testDeviceID1)
	}
	for _, invalid := range []string{"", "node_7E2AE42D46724E48B84021F72AFBE3DC", "7e2ae42d46724e48b84021f72afbe3dc"} {
		if _, err := DeviceIDFromNodeID(invalid); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("DeviceIDFromNodeID(%q) error = %v, want ErrInvalidMetadata", invalid, err)
		}
	}
}

func validMetadata(t *testing.T) Metadata {
	t.Helper()
	return Metadata{
		SchemaVersion: SchemaVersion,
		VaultID:       testVaultID,
		Revision:      1,
		CreatedAt:     "2026-09-22T10:00:00Z",
		UpdatedAt:     "2026-09-22T10:15:00Z",
		Devices:       []Device{validDevice(t, testDeviceID1, testNodeID1, "Gaming PC")},
	}
}

func validDevice(t *testing.T, deviceID, nodeID, name string) Device {
	t.Helper()
	identity, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	return Device{
		DeviceID:          deviceID,
		NodeID:            nodeID,
		Name:              name,
		IdentityPublicKey: e2ee.EncodeKey(identity.Public),
		RegisteredAt:      "2026-09-22T10:00:00Z",
	}
}

func mustJSONWithoutValidation(t *testing.T, metadata Metadata) []byte {
	t.Helper()
	// Keep this helper deliberately local to the test: production callers
	// cannot marshal invalid metadata through vaultmeta.Marshal.
	data := []byte(`{
  "schemaVersion": 2,
  "vaultId": "80cc57d6-03c4-4db4-bf2d-8c93a5ca4efa",
  "revision": 1,
  "createdAt": "2026-09-22T10:00:00Z",
  "updatedAt": "2026-09-22T10:15:00Z",
  "devices": []
}`)
	if metadata.SchemaVersion != 2 {
		t.Fatalf("mustJSONWithoutValidation only supports the schema-version test")
	}
	return data
}
