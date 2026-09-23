// Package protectedtokens stores future cloud-provider OAuth tokens outside
// SQLite. Windows uses the current user's Credential Manager; other platforms
// fail closed until an equivalent protected backend is implemented.
package protectedtokens

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound    = errors.New("protected cloud tokens not found")
	ErrUnsupported = errors.New("protected cloud token storage is unavailable on this platform")
)

const credentialPrefix = "GameSaveGo:OpenSave:cloud:"

// Tokens is one provider's rotating OAuth token pair. No account identifier,
// client secret, or game-save bytes belong in this record.
type Tokens struct {
	AccessToken     string `json:"accessToken"`
	RefreshToken    string `json:"refreshToken"`
	ExpiresAtUnixMs int64  `json:"expiresAtUnixMs"`
}

type envelope struct {
	Version  int    `json:"version"`
	Provider string `json:"provider"`
	Tokens   Tokens `json:"tokens"`
}

// Store is scoped to one provider and one local installation ID.
// The ID must be stable across app restarts and never contain an account ID.
type Store struct {
	provider string
	target   string
}

// New validates the non-secret identity used to name the OS credential.
func New(provider, installationID string) (*Store, error) {
	if !validPart(provider) || !validPart(installationID) {
		return nil, errors.New("invalid protected token store identity")
	}
	return &Store{provider: provider, target: credentialPrefix + provider + ":" + installationID}, nil
}

func validPart(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, ch := range s {
		if ch != '_' && ch != '-' && (ch < 'a' || ch > 'z') &&
			(ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

// Save replaces both tokens in one OS credential write. A missing token or
// expiry is rejected rather than persisting an apparently usable session.
func (s *Store) Save(tokens Tokens) error {
	if s == nil || s.target == "" {
		return errors.New("protected token store is not initialized")
	}
	if strings.TrimSpace(tokens.AccessToken) == "" || strings.TrimSpace(tokens.RefreshToken) == "" || tokens.ExpiresAtUnixMs <= 0 {
		return errors.New("incomplete cloud tokens")
	}
	raw, err := json.Marshal(envelope{Version: 1, Provider: s.provider, Tokens: tokens})
	if err != nil {
		return fmt.Errorf("encode protected cloud tokens: %w", err)
	}
	defer clear(raw)
	return writeCredential(s.target, raw)
}

// Load returns a validated token pair, or ErrNotFound if the user has not
// connected this provider. Corrupt records fail explicitly; they are not
// silently treated as a disconnected account.
func (s *Store) Load() (Tokens, error) {
	if s == nil || s.target == "" {
		return Tokens{}, errors.New("protected token store is not initialized")
	}
	raw, err := readCredential(s.target)
	if err != nil {
		return Tokens{}, err
	}
	defer clear(raw)
	var saved envelope
	if err := json.Unmarshal(raw, &saved); err != nil {
		return Tokens{}, errors.New("protected cloud tokens are corrupt")
	}
	if saved.Version != 1 || saved.Provider != s.provider ||
		strings.TrimSpace(saved.Tokens.AccessToken) == "" ||
		strings.TrimSpace(saved.Tokens.RefreshToken) == "" || saved.Tokens.ExpiresAtUnixMs <= 0 {
		return Tokens{}, errors.New("protected cloud tokens are invalid or from another provider")
	}
	return saved.Tokens, nil
}

// Delete removes this installation's provider credential without touching
// remote data. Deleting an absent record is safe and idempotent.
func (s *Store) Delete() error {
	if s == nil || s.target == "" {
		return errors.New("protected token store is not initialized")
	}
	return deleteCredential(s.target)
}
