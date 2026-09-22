// Package vaultmeta defines the provider-independent cloud-vault metadata and
// first-join preview contract. It deliberately contains no provider client and
// no write operation: discovering a vault must not be enough to mutate either
// the local saves or the remote library.
package vaultmeta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/opensave/opensave/internal/e2ee"
)

const (
	// SchemaVersion is the only vault.json version this build understands.
	SchemaVersion = 1
	maxDeviceName = 128
)

var (
	// ErrUnsupportedSchema tells callers to leave a newer remote vault alone.
	ErrUnsupportedSchema = errors.New("unsupported vault metadata schema")
	// ErrInvalidMetadata means the remote file is malformed or violates the
	// versioned contract. It is safe to show as a recoverable discovery error.
	ErrInvalidMetadata = errors.New("invalid vault metadata")

	legacyVaultIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	nodeIDPattern        = regexp.MustCompile(`^node_([0-9a-f]{32})$`)
)

// Metadata is the public, non-secret content of vault.json. Raw vault keys,
// private keys, provider identities, tokens, local paths, and saves never
// belong here.
type Metadata struct {
	SchemaVersion int      `json:"schemaVersion"`
	VaultID       string   `json:"vaultId"`
	Revision      uint64   `json:"revision"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
	Devices       []Device `json:"devices"`
}

// Device is one installation registered with the vault. NodeID preserves the
// existing P2P identity while DeviceID exposes the same 128 bits as a UUID.
type Device struct {
	DeviceID          string  `json:"deviceId"`
	NodeID            string  `json:"nodeId"`
	Name              string  `json:"name"`
	IdentityPublicKey string  `json:"identityPublicKey"`
	RegisteredAt      string  `json:"registeredAt"`
	RevokedAt         *string `json:"revokedAt"`
}

// Parse decodes and validates exactly one vault.json value. Unknown fields are
// rejected so a misspelled security-relevant property cannot be ignored.
func Parse(data []byte) (Metadata, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var metadata Metadata
	if err := dec.Decode(&metadata); err != nil {
		return Metadata{}, invalid("decode vault.json: %v", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Metadata{}, invalid("vault.json contains more than one value")
		}
		return Metadata{}, invalid("decode trailing vault.json data: %v", err)
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

// Marshal validates metadata and emits deterministic JSON. Devices are sorted
// on a copy, so callers do not observe their slice being reordered.
func Marshal(metadata Metadata) ([]byte, error) {
	if err := metadata.Validate(); err != nil {
		return nil, err
	}
	metadata.Devices = append([]Device(nil), metadata.Devices...)
	sort.Slice(metadata.Devices, func(i, j int) bool {
		return metadata.Devices[i].DeviceID < metadata.Devices[j].DeviceID
	})
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal vault metadata: %w", err)
	}
	return append(data, '\n'), nil
}

// Validate checks the complete version-1 contract without changing metadata.
func (metadata Metadata) Validate() error {
	switch {
	case metadata.SchemaVersion > SchemaVersion:
		return fmt.Errorf("%w: got %d, newest supported is %d", ErrUnsupportedSchema, metadata.SchemaVersion, SchemaVersion)
	case metadata.SchemaVersion != SchemaVersion:
		return invalid("schemaVersion must be %d", SchemaVersion)
	}
	if err := validateVaultID(metadata.VaultID); err != nil {
		return err
	}
	if metadata.Revision == 0 {
		return invalid("revision must be at least 1")
	}
	createdAt, err := parseUTCTimestamp("createdAt", metadata.CreatedAt)
	if err != nil {
		return err
	}
	updatedAt, err := parseUTCTimestamp("updatedAt", metadata.UpdatedAt)
	if err != nil {
		return err
	}
	if updatedAt.Before(createdAt) {
		return invalid("updatedAt precedes createdAt")
	}
	if len(metadata.Devices) == 0 {
		return invalid("devices must contain at least one registration")
	}

	seen := make(map[string]struct{}, len(metadata.Devices))
	for i, device := range metadata.Devices {
		if err := validateDevice(device); err != nil {
			return invalid("devices[%d]: %v", i, err)
		}
		if _, exists := seen[device.DeviceID]; exists {
			return invalid("devices contains duplicate deviceId %q", device.DeviceID)
		}
		seen[device.DeviceID] = struct{}{}
	}
	return nil
}

// DeviceIDFromNodeID losslessly renders the random 128-bit NodeID payload as
// the canonical UUID used by vault.json.
func DeviceIDFromNodeID(nodeID string) (string, error) {
	match := nodeIDPattern.FindStringSubmatch(nodeID)
	if match == nil {
		return "", invalid("nodeId must be node_ followed by 32 lowercase hexadecimal characters")
	}
	id, err := uuid.Parse(match[1])
	if err != nil {
		return "", invalid("nodeId payload is not a UUID: %v", err)
	}
	return id.String(), nil
}

func validateVaultID(vaultID string) error {
	if legacyVaultIDPattern.MatchString(vaultID) {
		return nil
	}
	id, err := uuid.Parse(vaultID)
	if err != nil || id.String() != vaultID || id.Version() != 4 {
		return invalid("vaultId must be a canonical UUID v4 or an exact 32-character lowercase legacy id")
	}
	return nil
}

func validateDevice(device Device) error {
	id, err := uuid.Parse(device.DeviceID)
	if err != nil || id.String() != device.DeviceID || id.Version() != 4 {
		return fmt.Errorf("deviceId must be a canonical UUID v4")
	}
	mapped, err := DeviceIDFromNodeID(device.NodeID)
	if err != nil {
		return err
	}
	if mapped != device.DeviceID {
		return fmt.Errorf("nodeId and deviceId do not identify the same installation")
	}
	if device.Name == "" || strings.TrimSpace(device.Name) != device.Name {
		return fmt.Errorf("name must be non-empty and have no surrounding whitespace")
	}
	if !utf8.ValidString(device.Name) || utf8.RuneCountInString(device.Name) > maxDeviceName {
		return fmt.Errorf("name must be valid UTF-8 and at most %d characters", maxDeviceName)
	}
	for _, r := range device.Name {
		if unicode.IsControl(r) {
			return fmt.Errorf("name must not contain control characters")
		}
	}
	if strings.TrimSpace(device.IdentityPublicKey) != device.IdentityPublicKey {
		return fmt.Errorf("identityPublicKey must not contain surrounding whitespace")
	}
	if _, err := e2ee.DecodeKey(device.IdentityPublicKey); err != nil {
		return fmt.Errorf("identityPublicKey is not a valid X25519 public key: %w", err)
	}
	registeredAt, err := parseUTCTimestamp("registeredAt", device.RegisteredAt)
	if err != nil {
		return err
	}
	if device.RevokedAt != nil {
		revokedAt, err := parseUTCTimestamp("revokedAt", *device.RevokedAt)
		if err != nil {
			return err
		}
		if revokedAt.Before(registeredAt) {
			return fmt.Errorf("revokedAt precedes registeredAt")
		}
	}
	return nil
}

func parseUTCTimestamp(field, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, invalid("%s must be RFC 3339: %v", field, err)
	}
	_, offset := parsed.Zone()
	if offset != 0 || !strings.HasSuffix(value, "Z") {
		return time.Time{}, invalid("%s must use UTC with a Z suffix", field)
	}
	return parsed, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidMetadata, fmt.Sprintf(format, args...))
}
