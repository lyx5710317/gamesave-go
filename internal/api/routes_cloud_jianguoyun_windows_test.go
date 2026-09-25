//go:build windows

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

func TestJianguoyunSettingsNeverEchoApplicationPassword(t *testing.T) {
	ts := startTestServer(t)
	const password = "test-only-app-password"
	resp, body := ts.do(t, http.MethodPost, "/api/settings", map[string]any{
		"cloudSync": map[string]any{
			"enabled": true, "provider": "jianguoyun", "url": store.JianguoyunBaseURL,
			"username": "test@example.invalid", "password": password,
		},
	})
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body["cloudSync"]), password) {
		t.Fatalf("settings write leaked or failed: status=%d leaked=%t", resp.StatusCode, strings.Contains(string(body["cloudSync"]), password))
	}
	t.Cleanup(func() { _ = ts.daemon.Store.DisconnectJianguoyun() })
	resp, body = ts.do(t, http.MethodGet, "/api/settings", nil)
	var config struct {
		Password           string `json:"password"`
		PasswordConfigured bool   `json:"passwordConfigured"`
	}
	if err := json.Unmarshal(body["cloudSync"], &config); err != nil || resp.StatusCode != http.StatusOK || config.Password != "" || !config.PasswordConfigured {
		t.Fatalf("settings read echoed a secret or lost configured state: status=%d err=%v configured=%t", resp.StatusCode, err, config.PasswordConfigured)
	}
	resp, _ = ts.do(t, http.MethodPost, "/api/settings", map[string]any{"cloudSync": map[string]any{"password": ""}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("blank update failed: %d", resp.StatusCode)
	}
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	if secret, err := ts.daemon.Store.LoadCloudPassword(cfg); err != nil || secret != password {
		t.Fatalf("blank settings update lost protected password: %v", err)
	}
	resp, _ = ts.do(t, http.MethodPost, "/api/cloud/jianguoyun/disconnect", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disconnect failed: %d", resp.StatusCode)
	}
	resp, body = ts.do(t, http.MethodGet, "/api/settings", nil)
	if err := json.Unmarshal(body["cloudSync"], &config); err != nil || config.PasswordConfigured {
		t.Fatalf("disconnect still reports a credential: err=%v", err)
	}
}
