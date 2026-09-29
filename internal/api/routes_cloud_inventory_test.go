package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/daemon"
)

type previewInventoryProvider struct {
	files  []cloud.CloudFile
	writes int
}

func (p *previewInventoryProvider) Upload(string, string) error {
	p.writes++
	return errors.New("unexpected upload")
}
func (p *previewInventoryProvider) List() ([]cloud.CloudFile, error) { return p.files, nil }
func (p *previewInventoryProvider) Download(string, string) error {
	p.writes++
	return errors.New("unexpected download")
}
func (p *previewInventoryProvider) Delete(cloud.CloudFile) error {
	p.writes++
	return errors.New("unexpected delete")
}

func TestCloudInventoryBrowseFailsClosedOnAmbiguousSnapshots(t *testing.T) {
	d, err := daemon.New(daemon.Options{HomeOverride: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	d.Scanner.ManifestURL = ""
	t.Cleanup(d.Stop)
	provider := &previewInventoryProvider{}
	if err := d.Cloud.RegisterProvider("preview_fixture", provider); err != nil {
		t.Fatal(err)
	}
	cfg, err := d.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider = true, "preview_fixture"
	if err := d.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	s := New(d)
	router := chi.NewRouter()
	s.cloudRoutes(router)
	check := func(path string, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != want {
			t.Fatalf("%s returned HTTP %d, want %d: %s", path, recorder.Code, want, recorder.Body.String())
		}
	}
	paths := []string{"/api/cloud/browse", "/api/cloud/snapshots/game"}
	for _, scenario := range []struct {
		name  string
		files []cloud.CloudFile
		want  int
	}{
		{"unique", []cloud.CloudFile{{Name: "game__main__snap.zip", SizeBytes: 12}}, http.StatusOK},
		{"duplicate", []cloud.CloudFile{
			{Name: "game__main__snap.zip", SizeBytes: 12, ID: "first"},
			{Name: "game__main__snap.zip", SizeBytes: 12, ID: "second"},
		}, http.StatusConflict},
		{"invalid size", []cloud.CloudFile{{Name: "game__main__snap.zip", SizeBytes: -1}}, http.StatusConflict},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			provider.files = scenario.files
			for _, path := range paths {
				check(path, scenario.want)
			}
		})
	}
	if provider.writes != 0 {
		t.Fatalf("read-only browse invoked %d writes", provider.writes)
	}
}
