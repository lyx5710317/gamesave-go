package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/opensave/opensave/internal/cloud/protectedtokens"
)

const JianguoyunBaseURL = "https://dav.jianguoyun.com/dav/"

// CloudConfig is the singleton cloud-backup configuration row, including
// the current provider's OAuth tokens. It mirrors the JS app's
// settings.cloudSync object, flattened (tokens.* -> top-level columns).
type CloudConfig struct {
	ID                  int               `db:"id" json:"-"`
	Enabled             bool              `db:"enabled" json:"enabled"`
	Provider            string            `db:"provider" json:"provider"`
	URL                 string            `db:"url" json:"url"`
	Username            string            `db:"username" json:"username"`
	Password            string            `db:"password" json:"password"`
	PasswordConfigured  bool              `db:"-" json:"passwordConfigured"`
	HeadersJSON         string            `db:"headers_json" json:"headers"`
	FolderID            string            `db:"folder_id" json:"folderId"`
	CustomClientIDs     map[string]string `db:"-" json:"customClientIds"`
	CustomClientSecrets map[string]string `db:"-" json:"customClientSecrets"`
	AccessToken         string            `db:"access_token" json:"-"`
	RefreshToken        string            `db:"refresh_token" json:"-"`
	ExpiryTimeMs        int64             `db:"expiry_time_ms" json:"-"`
	UserEmail           string            `db:"user_email" json:"-"`

	CustomClientIDsJSON     string `db:"custom_client_ids" json:"-"`
	CustomClientSecretsJSON string `db:"custom_client_secrets" json:"-"`
}

// GetCloudConfig returns the singleton cloud config row.
func (s *Store) GetCloudConfig() (CloudConfig, error) {
	var c CloudConfig
	if err := s.db.Get(&c, `SELECT * FROM cloud_config WHERE id = 1`); err != nil {
		return CloudConfig{}, fmt.Errorf("get cloud config: %w", err)
	}
	if protectedJianguoyunConfig(c) {
		secret, err := s.jianguoyunPasswordStore()
		if err != nil {
			return CloudConfig{}, err
		}
		if c.Password != "" {
			if existing, loadErr := secret.Load(); loadErr == nil && existing != c.Password {
				return CloudConfig{}, errors.New("legacy Jianguoyun password conflicts with an existing protected credential; migration stopped")
			} else if loadErr != nil && !errors.Is(loadErr, protectedtokens.ErrNotFound) {
				return CloudConfig{}, fmt.Errorf("read protected Jianguoyun password before migration: %w", loadErr)
			}
			// Upgrade legacy WebDAV/Jianguoyun rows before exposing settings or
			// using the destination. A failed OS write leaves SQLite unchanged.
			if err := replaceProtectedPassword(secret, c.Password, func() error {
				result, err := s.db.Exec(`UPDATE cloud_config SET password = '' WHERE id = 1 AND password = ?`, c.Password)
				if err != nil {
					return err
				}
				changed, err := result.RowsAffected()
				if err != nil || changed != 1 {
					return errors.New("Jianguoyun password changed concurrently during migration")
				}
				return nil
			}); err != nil {
				return CloudConfig{}, fmt.Errorf("protect existing Jianguoyun application password: %w", err)
			}
			c.Password = ""
		}
		_, err = secret.Load()
		if err != nil && !errors.Is(err, protectedtokens.ErrNotFound) {
			return CloudConfig{}, fmt.Errorf("read protected Jianguoyun password: %w", err)
		}
		c.PasswordConfigured = err == nil
	}
	if c.CustomClientIDsJSON != "" {
		if err := json.Unmarshal([]byte(c.CustomClientIDsJSON), &c.CustomClientIDs); err != nil {
			return CloudConfig{}, fmt.Errorf("unmarshal customClientIds: %w", err)
		}
	}
	if c.CustomClientSecretsJSON != "" {
		if err := json.Unmarshal([]byte(c.CustomClientSecretsJSON), &c.CustomClientSecrets); err != nil {
			return CloudConfig{}, fmt.Errorf("unmarshal customClientSecrets: %w", err)
		}
	}
	return c, nil
}

// UpdateCloudConfig persists the given config as the new singleton row.
func (s *Store) UpdateCloudConfig(c CloudConfig) error {
	var secret *protectedtokens.PasswordStore
	var newPassword string
	if protectedJianguoyunConfig(c) {
		if c.Provider == "jianguoyun" && c.URL != JianguoyunBaseURL {
			return errors.New("Jianguoyun preset requires its official WebDAV address")
		}
		if c.Provider == "webdav" {
			return errors.New("Jianguoyun WebDAV backups require the dedicated Jianguoyun preset")
		}
		var err error
		secret, err = s.jianguoyunPasswordStore()
		if err != nil {
			return err
		}
		newPassword = c.Password
		c.Password = "" // the SQLite column must never receive this secret
	}
	if c.CustomClientIDs == nil {
		c.CustomClientIDs = map[string]string{}
	}
	if c.CustomClientSecrets == nil {
		c.CustomClientSecrets = map[string]string{}
	}
	idsJSON, err := json.Marshal(c.CustomClientIDs)
	if err != nil {
		return err
	}
	secretsJSON, err := json.Marshal(c.CustomClientSecrets)
	if err != nil {
		return err
	}
	c.CustomClientIDsJSON = string(idsJSON)
	c.CustomClientSecretsJSON = string(secretsJSON)

	writeRow := func() error {
		_, err := s.db.NamedExec(`
		UPDATE cloud_config SET
			enabled = :enabled,
			provider = :provider,
			url = :url,
			username = :username,
			password = :password,
			headers_json = :headers_json,
			folder_id = :folder_id,
			custom_client_ids = :custom_client_ids,
			custom_client_secrets = :custom_client_secrets,
			access_token = :access_token,
			refresh_token = :refresh_token,
			expiry_time_ms = :expiry_time_ms,
			user_email = :user_email
		WHERE id = 1`, c)
		return err
	}
	if newPassword != "" {
		err = replaceProtectedPassword(secret, newPassword, writeRow)
	} else {
		err = writeRow()
	}
	if err != nil {
		return fmt.Errorf("update cloud config: %w", err)
	}
	return nil
}

// LoadCloudPassword is used only immediately before a Jianguoyun WebDAV
// request. Settings APIs never receive the application password.
func (s *Store) LoadCloudPassword(c CloudConfig) (string, error) {
	if !protectedJianguoyunConfig(c) {
		return c.Password, nil
	}
	secret, err := s.jianguoyunPasswordStore()
	if err != nil {
		return "", err
	}
	return secret.Load()
}

// DisconnectJianguoyun removes only the local application password. A failed
// SQLite update restores the credential; no remote files are deleted.
func (s *Store) DisconnectJianguoyun() error {
	c, err := s.GetCloudConfig()
	if err != nil {
		return err
	}
	if !protectedJianguoyunConfig(c) {
		return errors.New("Jianguoyun is not the selected WebDAV destination")
	}
	secret, err := s.jianguoyunPasswordStore()
	if err != nil {
		return err
	}
	old, err := secret.Load()
	if err != nil && !errors.Is(err, protectedtokens.ErrNotFound) {
		return err
	}
	if err := secret.Delete(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE cloud_config SET enabled = 0, username = '', password = '' WHERE id = 1`); err != nil {
		if old != "" {
			if restoreErr := secret.Save(old); restoreErr != nil {
				return errors.Join(err, restoreErr)
			}
		}
		return err
	}
	return nil
}

func protectedJianguoyunConfig(c CloudConfig) bool {
	return c.Provider == "jianguoyun" || (c.Provider == "webdav" && IsJianguoyunHost(c.URL))
}

// IsJianguoyunHost detects legacy generic-WebDAV destinations at the official
// host so their passwords can be migrated before settings are displayed.
func IsJianguoyunHost(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(u.Hostname(), "dav.jianguoyun.com")
}

func (s *Store) jianguoyunPasswordStore() (*protectedtokens.PasswordStore, error) {
	var nodeID string
	if err := s.db.Get(&nodeID, `SELECT node_id FROM settings WHERE id = 1`); err != nil {
		return nil, fmt.Errorf("read local installation identity: %w", err)
	}
	return protectedtokens.NewPasswordStore("jianguoyun", nodeID)
}

func replaceProtectedPassword(secret *protectedtokens.PasswordStore, password string, writeRow func() error) error {
	old, err := secret.Load()
	if err != nil && !errors.Is(err, protectedtokens.ErrNotFound) {
		return err
	}
	if err := secret.Save(password); err != nil {
		return err
	}
	if err := writeRow(); err != nil {
		var rollbackErr error
		if old == "" {
			rollbackErr = secret.Delete()
		} else {
			rollbackErr = secret.Save(old)
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}
