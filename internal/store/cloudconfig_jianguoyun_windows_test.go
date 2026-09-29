//go:build windows

package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/cloud/protectedtokens"
)

func testJianguoyunCredential(t *testing.T, s *Store) *protectedtokens.PasswordStore {
	t.Helper()
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	secret, err := s.jianguoyunPasswordStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secret.Delete() })
	return secret
}

func TestJianguoyunPasswordWriteReadBlankUpdateAndDisconnect(t *testing.T) {
	s := openTestStore(t)
	secret := testJianguoyunCredential(t, s)
	c, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Provider, c.URL, c.Username, c.Password = "jianguoyun", JianguoyunBaseURL, "test@example.invalid", "test-app-password"
	if err := s.UpdateCloudConfig(c); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.db.Get(&raw, `SELECT password FROM cloud_config WHERE id = 1`); err != nil || raw != "" {
		t.Fatalf("SQLite retained an application password: err=%v, nonempty=%t", err, raw != "")
	}
	stored, err := s.GetCloudConfig()
	if err != nil || stored.Password != "" || !stored.PasswordConfigured {
		t.Fatalf("masked settings invalid: err=%v configured=%t password-empty=%t", err, stored.PasswordConfigured, stored.Password == "")
	}
	if value, err := s.LoadCloudPassword(stored); err != nil || value != "test-app-password" {
		t.Fatalf("protected password did not round trip: %v", err)
	}
	stored.Password = "" // the UI sends an empty field on unrelated saves
	if err := s.UpdateCloudConfig(stored); err != nil {
		t.Fatal(err)
	}
	if value, err := secret.Load(); err != nil || value != "test-app-password" {
		t.Fatalf("empty update cleared the credential: %v", err)
	}
	if err := s.DisconnectJianguoyun(); err != nil {
		t.Fatal(err)
	}
	if _, err := secret.Load(); !errors.Is(err, protectedtokens.ErrNotFound) {
		t.Fatalf("explicit disconnect retained credential: %v", err)
	}
	stored, err = s.GetCloudConfig()
	if err != nil || stored.Enabled || stored.PasswordConfigured || stored.Username != "" {
		t.Fatalf("disconnect did not clear local connection metadata: %+v, %v", stored, err)
	}
}

func TestLegacyJianguoyunWebDAVPasswordMigratesAndRollsBack(t *testing.T) {
	for _, failDB := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[failDB], func(t *testing.T) {
			s := openTestStore(t)
			secret := testJianguoyunCredential(t, s)
			if _, err := s.db.Exec(`UPDATE cloud_config SET provider='webdav', url=?, password=? WHERE id=1`, JianguoyunBaseURL, "legacy-test-password"); err != nil {
				t.Fatal(err)
			}
			if failDB {
				if _, err := s.db.Exec(`CREATE TRIGGER fail_cloud_password_clear BEFORE UPDATE ON cloud_config WHEN NEW.password = '' BEGIN SELECT RAISE(FAIL, 'simulated failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := s.GetCloudConfig()
			var raw string
			if getErr := s.db.Get(&raw, `SELECT password FROM cloud_config WHERE id=1`); getErr != nil {
				t.Fatal(getErr)
			}
			if failDB {
				if err == nil || raw != "legacy-test-password" {
					t.Fatalf("failed migration discarded plaintext before secure commit: err=%v retained=%t", err, raw == "legacy-test-password")
				}
				if _, err := secret.Load(); !errors.Is(err, protectedtokens.ErrNotFound) {
					t.Fatalf("failed migration left a new credential: %v", err)
				}
				return
			}
			if err != nil || raw != "" || !cfg.PasswordConfigured || cfg.Password != "" {
				t.Fatalf("migration failed to clear SQLite password: err=%v raw-empty=%t configured=%t", err, raw == "", cfg.PasswordConfigured)
			}
			if value, err := s.LoadCloudPassword(cfg); err != nil || value != "legacy-test-password" {
				t.Fatalf("migrated password unavailable: %v", err)
			}
		})
	}
}

func TestLegacyJianguoyunMigrationRejectsDifferentExistingCredential(t *testing.T) {
	s := openTestStore(t)
	secret := testJianguoyunCredential(t, s)
	if err := secret.Save("newer-protected-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE cloud_config SET provider='webdav', url=?, password=? WHERE id=1`, JianguoyunBaseURL, "older-plaintext-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCloudConfig(); err == nil {
		t.Fatal("conflicting legacy password silently replaced a protected credential")
	}
	if value, err := secret.Load(); err != nil || value != "newer-protected-password" {
		t.Fatalf("existing protected credential changed: %v", err)
	}
}

func TestJianguoyunDisconnectRestoresCredentialWhenSQLiteFails(t *testing.T) {
	s := openTestStore(t)
	secret := testJianguoyunCredential(t, s)
	c, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Provider, c.URL, c.Password = "jianguoyun", JianguoyunBaseURL, "keep-on-failure"
	if err := s.UpdateCloudConfig(c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_disconnect BEFORE UPDATE ON cloud_config WHEN NEW.enabled = 0 BEGIN SELECT RAISE(FAIL, 'simulated failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.DisconnectJianguoyun(); err == nil {
		t.Fatal("failed SQLite disconnect was reported as success")
	}
	if value, err := secret.Load(); err != nil || value != "keep-on-failure" {
		t.Fatalf("failed disconnect discarded protected credential: %v", err)
	}
}

func TestJianguoyunCredentialWriteFailureNeverFallsBackToPlaintext(t *testing.T) {
	s := openTestStore(t)
	_ = testJianguoyunCredential(t, s)
	c, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Provider, c.URL, c.Password = "jianguoyun", JianguoyunBaseURL, strings.Repeat("x", 3000)
	if err := s.UpdateCloudConfig(c); err == nil {
		t.Fatal("oversized protected credential was accepted")
	}
	var raw string
	if err := s.db.Get(&raw, `SELECT password FROM cloud_config WHERE id=1`); err != nil || raw != "" {
		t.Fatalf("failed protected write fell back to plaintext: err=%v nonempty=%t", err, raw != "")
	}
}

func TestOfficialJianguoyunHostRequiresProtectedPreset(t *testing.T) {
	s := openTestStore(t)
	secret := testJianguoyunCredential(t, s)
	c, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Provider, c.URL, c.Password = "webdav", "http://dav.jianguoyun.com/dav/", "test-app-password"
	if err := s.UpdateCloudConfig(c); err == nil {
		t.Fatal("official host accepted an insecure WebDAV address")
	}
	var raw string
	if err := s.db.Get(&raw, `SELECT password FROM cloud_config WHERE id=1`); err != nil || raw != "" {
		t.Fatalf("insecure URL persisted plaintext: err=%v nonempty=%t", err, raw != "")
	}
	if _, err := secret.Load(); !errors.Is(err, protectedtokens.ErrNotFound) {
		t.Fatalf("rejected update retained a new credential: %v", err)
	}
	c.URL = JianguoyunBaseURL
	if err := s.UpdateCloudConfig(c); err == nil {
		t.Fatal("generic WebDAV accepted the official host and bypassed the preset")
	}
	c.URL = "http://dav.jianguoyun.com/dav/"
	// A pre-existing insecure row is still masked and migrated before the UI
	// can read it; saving it again requires correcting the URL.
	if _, err := s.db.Exec(`UPDATE cloud_config SET provider='webdav', url=?, password=? WHERE id=1`, c.URL, c.Password); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.GetCloudConfig()
	if err != nil || legacy.Password != "" || !legacy.PasswordConfigured {
		t.Fatalf("legacy insecure credential was exposed: %+v, %v", legacy, err)
	}
	if err := s.UpdateCloudConfig(legacy); err == nil {
		t.Fatal("insecure legacy URL remained saveable")
	}
}
