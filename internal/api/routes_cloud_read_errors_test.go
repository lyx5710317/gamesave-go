package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/cloud"
)

func TestCloudReadErrorsAreFixedAndPrivateAcrossBothEndpoints(t *testing.T) {
	ts := startTestServer(t)
	provider := &verifyFailureProvider{}
	if err := ts.daemon.Cloud.RegisterProvider("read_failure", provider); err != nil {
		t.Fatal(err)
	}
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider = true, "read_failure"
	if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	New(ts.daemon).cloudRoutes(router)
	for _, tc := range []struct {
		code string
		err  error
	}{
		{"configuration", errors.New("application password is not configured")},
		{"authentication", cloud.ErrJianguoyunAuth},
		{"permission", cloud.ErrJianguoyunPermission},
		{"quota", cloud.ErrJianguoyunQuota},
		{"rate_limit", cloud.ErrJianguoyunRateLimit},
		{"network", cloud.ErrJianguoyunNetwork},
		{"missing", cloud.ErrJianguoyunMissing},
		{"incomplete_inventory", cloud.ErrJianguoyunIncomplete},
		{"ambiguous", cloud.ErrRemoteSnapshotAmbiguous},
		{"failed", errors.New("untrusted provider response")},
	} {
		provider.listErr = fmt.Errorf("synthetic-private-detail user@example.invalid Authorization: synthetic: %w", tc.err)
		for _, path := range []string{"/api/cloud/browse", "/api/cloud/snapshots/game"} {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != 502 || body["code"] != "cloud_read_"+tc.code ||
				body["error"] != "cloud inventory unavailable; no saves were changed" || len(body) != 2 {
				t.Fatalf("incorrect read diagnostic: %d, %v", recorder.Code, body)
			}
			for _, forbidden := range []string{"synthetic", "example.invalid", "Authorization"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("private read error leaked: %s", forbidden)
				}
			}
		}
	}
}

func TestCloudReadDisabledAndInvalidInventoryCannotBecomeEmptySuccess(t *testing.T) {
	ts := startTestServer(t)
	provider := &previewInventoryProvider{}
	if err := ts.daemon.Cloud.RegisterProvider("read_state", provider); err != nil {
		t.Fatal(err)
	}
	cfg, err := ts.daemon.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Provider = "read_state"
	router := chi.NewRouter()
	New(ts.daemon).cloudRoutes(router)
	for _, tc := range []struct {
		name    string
		enabled bool
		files   []cloud.CloudFile
		status  int
		code    string
	}{
		{"disabled", false, nil, 502, "cloud_read_disabled"},
		{"duplicates", true, []cloud.CloudFile{{Name: "game__main__snap.zip"}, {Name: "game__main__snap.zip"}}, 409, "cloud_read_incomplete_inventory"},
		{"invalid size", true, []cloud.CloudFile{{Name: "game__main__snap.zip", SizeBytes: -1}}, 409, "cloud_read_incomplete_inventory"},
		{"known empty", true, nil, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg.Enabled = tc.enabled
			if err := ts.daemon.Store.UpdateCloudConfig(cfg); err != nil {
				t.Fatal(err)
			}
			provider.files = tc.files
			for _, path := range []string{"/api/cloud/browse", "/api/cloud/snapshots/game"} {
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
				if recorder.Code != tc.status {
					t.Fatalf("incorrect status: %d", recorder.Code)
				}
				if tc.code != "" {
					var body map[string]string
					if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					if body["code"] != tc.code {
						t.Fatalf("incorrect code: %v", body)
					}
				} else if strings.TrimSpace(recorder.Body.String()) != "[]" {
					t.Fatalf("expected empty inventory: %s", recorder.Body.String())
				}
			}
		})
	}
	if provider.writes != 0 {
		t.Fatalf("browse performed %d writes", provider.writes)
	}
}
