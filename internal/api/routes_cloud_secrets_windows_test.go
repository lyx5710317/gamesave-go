//go:build windows

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCloudSettingsMaskAdditionalSecretsAndPreserveBlankFields(t *testing.T) {
	ts := startTestServer(t)
	const password = "test-webdav-password"
	const header = "test-header-token"
	const client = "test-client-secret"
	resp, body := ts.do(t, http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{
			"provider": "webdav", "url": "https://example.invalid/dav/", "password": password,
			"headers":             `{"Authorization":"Bearer ` + header + `"}`,
			"customClientSecrets": map[string]string{"google_drive": client},
		},
	})
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body["cloudSync"]), password) || strings.Contains(string(body["cloudSync"]), header) || strings.Contains(string(body["cloudSync"]), client) {
		t.Fatalf("write response failed or exposed a secret: status=%d", resp.StatusCode)
	}
	assertMasked := func() {
		t.Helper()
		resp, body = ts.do(t, http.MethodGet, "/api/settings", nil)
		var cfg struct {
			Password                      string          `json:"password"`
			PasswordConfigured            bool            `json:"passwordConfigured"`
			Headers                       string          `json:"headers"`
			HeadersConfigured             bool            `json:"headersConfigured"`
			CustomClientSecrets           json.RawMessage `json:"customClientSecrets"`
			CustomClientSecretsConfigured map[string]bool `json:"customClientSecretsConfigured"`
		}
		if err := json.Unmarshal(body["cloudSync"], &cfg); err != nil || resp.StatusCode != http.StatusOK || cfg.Password != "" || !cfg.PasswordConfigured || cfg.Headers != "{}" || !cfg.HeadersConfigured || string(cfg.CustomClientSecrets) != "{}" || !cfg.CustomClientSecretsConfigured["google_drive"] {
			t.Fatalf("settings did not mask protected fields: status=%d err=%v", resp.StatusCode, err)
		}
	}
	assertMasked()
	resp, _ = ts.do(t, http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{
			"password": "", "headers": "{}", "customClientSecrets": map[string]string{"google_drive": ""},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("blank update failed: %d", resp.StatusCode)
	}
	assertMasked()
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ts.daemon.Store.LoadCloudPassword(cfg); err != nil || got != password || !strings.Contains(cfg.HeadersJSON, header) || cfg.CustomClientSecrets["google_drive"] != client {
		t.Fatalf("blank update lost a protected secret: %v", err)
	}
	resp, _ = ts.do(t, http.MethodPost, "/api/settings", map[string]any{"cloudSync": map[string]any{"clearHeaders": true}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("explicit header removal failed: %d", resp.StatusCode)
	}
	cfg, err = ts.daemon.Store.GetCloudConfig()
	if err != nil || cfg.HeadersConfigured {
		t.Fatalf("explicit header removal retained a credential: %v", err)
	}
}
