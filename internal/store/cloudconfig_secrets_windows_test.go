//go:build windows

package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/cloud/protectedtokens"
)

func testCloudSecret(t *testing.T, s *Store, kind string) *protectedtokens.PasswordStore {
	t.Helper()
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	secret, err := s.cloudSecretStore(kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secret.Delete() })
	return secret
}

func TestAdditionalCloudSecretsProtectedAndBlankUpdatePreserved(t *testing.T) {
	s := openTestStore(t)
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	webdav, err := s.webDAVPasswordStore("https://example.invalid/dav/")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = webdav.Delete() })
	clients := testCloudSecret(t, s, "client-secrets")
	headers := testCloudSecret(t, s, "request-headers")
	cfg, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Provider, cfg.URL, cfg.Username, cfg.Password = "webdav", "https://example.invalid/dav/", "test-user", "test-password"
	cfg.CustomClientSecrets = map[string]string{"google_drive": "test-client-secret"}
	cfg.HeadersJSON = `{"Authorization":"Bearer test-header"}`
	if err := s.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	var password, clientJSON, headerJSON string
	if err := s.db.QueryRow(`SELECT password, custom_client_secrets, headers_json FROM cloud_config WHERE id=1`).Scan(&password, &clientJSON, &headerJSON); err != nil {
		t.Fatal(err)
	}
	if password != "" || clientJSON != "{}" || headerJSON != "{}" {
		t.Fatalf("SQLite retained a secret: password=%t client=%t headers=%t", password != "", clientJSON != "{}", headerJSON != "{}")
	}
	got, err := s.GetCloudConfig()
	if err != nil || got.Password != "" || !got.PasswordConfigured || !got.HeadersConfigured || got.CustomClientSecrets["google_drive"] != "test-client-secret" {
		t.Fatalf("protected cloud config read failed: err=%v passwordConfigured=%t headersConfigured=%t", err, got.PasswordConfigured, got.HeadersConfigured)
	}
	got.Password = ""
	if err := s.UpdateCloudConfig(got); err != nil {
		t.Fatal(err)
	}
	if value, err := s.LoadCloudPassword(got); err != nil || value != "test-password" {
		t.Fatalf("blank update lost WebDAV password: %v", err)
	}
	if value, err := clients.Load(); err != nil || !strings.Contains(value, "test-client-secret") {
		t.Fatalf("blank update lost client secret: %v", err)
	}
	if value, err := headers.Load(); err != nil || !strings.Contains(value, "test-header") {
		t.Fatalf("blank update lost headers: %v", err)
	}
	got.CustomClientSecrets = map[string]string{}
	got.HeadersJSON = "{}"
	if err := s.UpdateCloudConfig(got); err != nil {
		t.Fatal(err)
	}
	if _, err := clients.Load(); !errors.Is(err, protectedtokens.ErrNotFound) {
		t.Fatalf("explicit client secret removal failed: %v", err)
	}
	if _, err := headers.Load(); !errors.Is(err, protectedtokens.ErrNotFound) {
		t.Fatalf("explicit header removal failed: %v", err)
	}
}

func TestLegacyAdditionalSecretsMigrationRollsBackOnSQLiteFailure(t *testing.T) {
	s := openTestStore(t)
	client := testCloudSecret(t, s, "client-secrets")
	if _, err := s.db.Exec(`UPDATE cloud_config SET custom_client_secrets=? WHERE id=1`, `{"google_drive":"legacy-test-secret"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_client_clear BEFORE UPDATE ON cloud_config WHEN NEW.custom_client_secrets='{}' BEGIN SELECT RAISE(FAIL, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCloudConfig(); err == nil {
		t.Fatal("failed migration reported success")
	}
	var raw string
	if err := s.db.Get(&raw, `SELECT custom_client_secrets FROM cloud_config WHERE id=1`); err != nil || !strings.Contains(raw, "legacy-test-secret") {
		t.Fatalf("failed migration lost original row: %v", err)
	}
	if _, err := client.Load(); !errors.Is(err, protectedtokens.ErrNotFound) {
		t.Fatalf("failed migration left a credential: %v", err)
	}
}

func TestAdditionalSecretWriteFailureDoesNotFallBackToSQLite(t *testing.T) {
	s := openTestStore(t)
	_ = testCloudSecret(t, s, "request-headers")
	cfg, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.HeadersJSON = `{"Authorization":"` + strings.Repeat("x", 3000) + `"}`
	if err := s.UpdateCloudConfig(cfg); err == nil {
		t.Fatal("oversized protected header was accepted")
	}
	var raw string
	if err := s.db.Get(&raw, `SELECT headers_json FROM cloud_config WHERE id=1`); err != nil || raw != "{}" {
		t.Fatalf("failed write persisted plaintext headers: %v", err)
	}
}

func TestWebDAVPasswordIsBoundToItsDestination(t *testing.T) {
	s := openTestStore(t)
	if err := s.EnsureDefaultSettings(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Provider, cfg.URL, cfg.Password = "webdav", "https://first.example.invalid/dav/", "first-destination-only"
	first, err := s.webDAVPasswordStore(cfg.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Delete() })
	if err := s.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg, err = s.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.URL = "https://second.example.invalid/dav/"
	if password, err := s.LoadCloudPassword(cfg); err != nil || password != "" {
		t.Fatalf("old credential sent to a different WebDAV server: %v", err)
	}
}
