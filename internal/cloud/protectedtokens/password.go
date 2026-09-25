package protectedtokens

import (
	"encoding/json"
	"errors"
	"strings"
)

// PasswordStore holds one provider-specific application password in the same
// OS credential backend as OAuth tokens, but under a separate target name.
type PasswordStore struct {
	provider string
	target   string
}

type passwordEnvelope struct {
	Version  int    `json:"version"`
	Provider string `json:"provider"`
	Password string `json:"password"`
}

func NewPasswordStore(provider, installationID string) (*PasswordStore, error) {
	if !validPart(provider) || !validPart(installationID) {
		return nil, errors.New("invalid protected password store identity")
	}
	return &PasswordStore{provider: provider, target: credentialPrefix + "password:" + provider + ":" + installationID}, nil
}

func (s *PasswordStore) Save(password string) error {
	if s == nil || s.target == "" || strings.TrimSpace(password) == "" {
		return errors.New("empty or uninitialized protected password")
	}
	raw, err := json.Marshal(passwordEnvelope{Version: 1, Provider: s.provider, Password: password})
	if err != nil {
		return errors.New("encode protected password")
	}
	defer clear(raw)
	return writeCredential(s.target, raw)
}

func (s *PasswordStore) Load() (string, error) {
	if s == nil || s.target == "" {
		return "", errors.New("uninitialized protected password store")
	}
	raw, err := readCredential(s.target)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	var saved passwordEnvelope
	if json.Unmarshal(raw, &saved) != nil || saved.Version != 1 || saved.Provider != s.provider || strings.TrimSpace(saved.Password) == "" {
		return "", errors.New("protected password is corrupt or belongs to another provider")
	}
	return saved.Password, nil
}

func (s *PasswordStore) Delete() error {
	if s == nil || s.target == "" {
		return errors.New("uninitialized protected password store")
	}
	return deleteCredential(s.target)
}
