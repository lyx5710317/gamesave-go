package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/daemon"
	"github.com/opensave/opensave/internal/e2ee"
	"github.com/opensave/opensave/internal/vaultmeta"
)

func TestCloudRemoteVaultPreviewIsReadOnlyAndDoesNotExposeMetadata(t *testing.T) {
	d, err := daemon.New(daemon.Options{HomeOverride: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	d.Scanner.ManifestURL = ""
	t.Cleanup(d.Stop)
	s := New(d)
	router := chi.NewRouter()
	s.cloudRoutes(router)
	dir := t.TempDir()
	cfg, err := d.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider, cfg.URL = true, "local", dir
	if err := d.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}

	check := func() (int, map[string]any) {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/cloud/join/remote-vault", nil))
		var result map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return recorder.Code, result
	}

	if status, result := check(); status != http.StatusOK || result["status"] != "missing" {
		t.Fatalf("missing vault: HTTP %d, %v", status, result)
	}
	path := filepath.Join(dir, "vault.json")
	// Deliberately invalid sensitive-looking bytes must not reach the response.
	if err := os.WriteFile(path, []byte(`{"secret":"do-not-echo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, result := check(); status != http.StatusOK || result["status"] != "invalid" || len(result) != 1 {
		t.Fatalf("invalid vault: HTTP %d, %v", status, result)
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, result := check(); status != http.StatusOK || result["status"] != "upgrade-required" {
		t.Fatalf("newer vault: HTTP %d, %v", status, result)
	}
	identity, err := e2ee.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	valid := vaultmeta.Metadata{
		SchemaVersion: 1,
		VaultID:       "550e8400-e29b-41d4-a716-446655440000",
		Revision:      3,
		CreatedAt:     "2026-09-22T10:00:00Z",
		UpdatedAt:     "2026-09-22T10:00:00Z",
		Devices: []vaultmeta.Device{{
			DeviceID:          "550e8400-e29b-41d4-a716-446655440001",
			NodeID:            "node_550e8400e29b41d4a716446655440001",
			Name:              "Private device name",
			IdentityPublicKey: e2ee.EncodeKey(identity.Public),
			RegisteredAt:      "2026-09-22T10:00:00Z",
		}},
	}
	raw, err := vaultmeta.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if status, result := check(); status != http.StatusOK || result["status"] != "valid" ||
		result["revision"] != float64(3) || result["deviceCount"] != float64(1) || len(result) != 3 {
		t.Fatalf("valid vault leaked metadata or returned wrong status: HTTP %d, %v", status, result)
	}
	if err := os.WriteFile(path, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Provider = "webhook"
	if err := d.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if status, result := check(); status != http.StatusOK || result["status"] != "unsupported" {
		t.Fatalf("unsupported provider: HTTP %d, %v", status, result)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("inspection modified remote fixture: %q, %v", content, err)
	}
}
