//go:build windows

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/opensave/opensave/internal/cloud/protectedtokens"
)

// The generic WebDAV credential is bound to its exact destination. Changing
// the URL cannot silently send an old password to a different server.
func (s *Store) webDAVPasswordStore(rawURL string) (*protectedtokens.PasswordStore, error) {
	sum := sha256.Sum256([]byte(rawURL))
	return s.cloudSecretStore("webdav-" + hex.EncodeToString(sum[:16]))
}

func (s *Store) cloudSecretStore(kind string) (*protectedtokens.PasswordStore, error) {
	var nodeID string
	if err := s.db.Get(&nodeID, `SELECT node_id FROM settings WHERE id = 1`); err != nil {
		return nil, fmt.Errorf("read local installation identity: %w", err)
	}
	return protectedtokens.NewPasswordStore(kind, nodeID)
}

func (s *Store) loadAdditionalCloudSecrets(c *CloudConfig) error {
	if c.Provider == "webdav" && !IsJianguoyunHost(c.URL) {
		secret, err := s.webDAVPasswordStore(c.URL)
		if err != nil {
			return err
		}
		if c.Password != "" {
			if err := s.migrateCloudSecret(secret, c.Password, `UPDATE cloud_config SET password = '' WHERE id = 1 AND password = ?`); err != nil {
				return fmt.Errorf("protect existing WebDAV password: %w", err)
			}
			c.Password = ""
		}
		_, err = secret.Load()
		if err != nil && !errors.Is(err, protectedtokens.ErrNotFound) {
			return fmt.Errorf("read protected WebDAV password: %w", err)
		}
		c.PasswordConfigured = err == nil
	}

	clientSecrets, err := s.cloudSecretStore("client-secrets")
	if err != nil {
		return err
	}
	if hasNonemptySecret(c.CustomClientSecrets) {
		legacy := c.CustomClientSecretsJSON
		if err := s.migrateCloudSecret(clientSecrets, legacy,
			`UPDATE cloud_config SET custom_client_secrets = '{}' WHERE id = 1 AND custom_client_secrets = ?`); err != nil {
			return fmt.Errorf("protect existing OAuth client secrets: %w", err)
		}
		c.CustomClientSecretsJSON = "{}"
	}
	if raw, err := clientSecrets.Load(); err == nil {
		var values map[string]string
		if json.Unmarshal([]byte(raw), &values) != nil || values == nil {
			return errors.New("protected OAuth client secrets are invalid")
		}
		c.CustomClientSecrets = values
	} else if !errors.Is(err, protectedtokens.ErrNotFound) {
		return fmt.Errorf("read protected OAuth client secrets: %w", err)
	}

	headers, err := s.cloudSecretStore("request-headers")
	if err != nil {
		return err
	}
	if nonemptyHeaders(c.HeadersJSON) {
		legacy := c.HeadersJSON
		if err := s.migrateCloudSecret(headers, legacy,
			`UPDATE cloud_config SET headers_json = '{}' WHERE id = 1 AND headers_json = ?`); err != nil {
			return fmt.Errorf("protect existing cloud request headers: %w", err)
		}
		c.HeadersJSON = "{}"
	}
	if raw, err := headers.Load(); err == nil {
		c.HeadersJSON = raw
		c.HeadersConfigured = true
	} else if !errors.Is(err, protectedtokens.ErrNotFound) {
		return fmt.Errorf("read protected cloud request headers: %w", err)
	}
	return nil
}

func (s *Store) migrateCloudSecret(secret *protectedtokens.PasswordStore, legacy, updateSQL string) error {
	if existing, err := secret.Load(); err == nil && existing != legacy {
		return errors.New("legacy plaintext conflicts with an existing protected credential")
	} else if err != nil && !errors.Is(err, protectedtokens.ErrNotFound) {
		return err
	}
	return replaceProtectedPassword(secret, legacy, func() error {
		result, err := s.db.Exec(updateSQL, legacy)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return errors.New("cloud credential changed concurrently during migration")
		}
		return nil
	})
}

func (s *Store) loadAdditionalCloudPassword(c CloudConfig) (string, error) {
	if c.Provider != "webdav" || IsJianguoyunHost(c.URL) {
		return c.Password, nil
	}
	secret, err := s.webDAVPasswordStore(c.URL)
	if err != nil {
		return "", err
	}
	password, err := secret.Load()
	if errors.Is(err, protectedtokens.ErrNotFound) {
		// Generic WebDAV may be intentionally unauthenticated.
		return "", nil
	}
	return password, err
}

type credentialChange struct {
	store *protectedtokens.PasswordStore
	value string
	old   string
	apply bool
}

func (s *Store) protectAdditionalCloudSecrets(c *CloudConfig, writeRow func() error) error {
	var changes []credentialChange
	if c.Provider == "webdav" && !IsJianguoyunHost(c.URL) && c.Password != "" {
		secret, err := s.webDAVPasswordStore(c.URL)
		if err != nil {
			return err
		}
		changes = append(changes, credentialChange{store: secret, value: c.Password})
		c.Password = ""
	}
	clientSecrets, err := s.cloudSecretStore("client-secrets")
	if err != nil {
		return err
	}
	value := ""
	if hasNonemptySecret(c.CustomClientSecrets) {
		value = c.CustomClientSecretsJSON
	}
	changes = append(changes, credentialChange{store: clientSecrets, value: value})
	c.CustomClientSecretsJSON = "{}"

	headers, err := s.cloudSecretStore("request-headers")
	if err != nil {
		return err
	}
	value = ""
	if nonemptyHeaders(c.HeadersJSON) {
		value = c.HeadersJSON
	}
	changes = append(changes, credentialChange{store: headers, value: value})
	c.HeadersJSON = "{}"

	rollback := func(last int) error {
		var result error
		for i := last; i >= 0; i-- {
			change := changes[i]
			if !change.apply {
				continue
			}
			if change.old == "" {
				result = errors.Join(result, change.store.Delete())
			} else {
				result = errors.Join(result, change.store.Save(change.old))
			}
		}
		return result
	}
	for i := range changes {
		old, err := changes[i].store.Load()
		if err != nil && !errors.Is(err, protectedtokens.ErrNotFound) {
			return errors.Join(err, rollback(i-1))
		}
		changes[i].old = old
		if old == changes[i].value {
			continue
		}
		if changes[i].value == "" {
			err = changes[i].store.Delete()
		} else {
			err = changes[i].store.Save(changes[i].value)
		}
		if err != nil {
			return errors.Join(err, rollback(i-1))
		}
		changes[i].apply = true
	}
	if err := writeRow(); err != nil {
		return errors.Join(err, rollback(len(changes)-1))
	}
	return nil
}

func hasNonemptySecret(values map[string]string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func nonemptyHeaders(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed != "" && trimmed != "{}"
}
