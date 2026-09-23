package daemon

import (
	"errors"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/cloud"
)

type failingCloudUpload struct{}

func (failingCloudUpload) Upload(_, _ string) error         { return errors.New("access_token=secret-value") }
func (failingCloudUpload) List() ([]cloud.CloudFile, error) { return nil, nil }
func (failingCloudUpload) Download(_, _ string) error       { return nil }
func (failingCloudUpload) Delete(cloud.CloudFile) error     { return nil }

type conflictingCloudUpload struct{ uploads int }

func (p *conflictingCloudUpload) Upload(_, _ string) error { p.uploads++; return nil }
func (p *conflictingCloudUpload) List() ([]cloud.CloudFile, error) {
	return []cloud.CloudFile{{Name: "game__main__snap.zip"}}, nil
}
func (p *conflictingCloudUpload) Download(_, _ string) error   { return nil }
func (p *conflictingCloudUpload) Delete(cloud.CloudFile) error { return nil }

func TestAutomaticCloudUploadStopsOnRemoteNameCollision(t *testing.T) {
	d, err := New(Options{HomeOverride: t.TempDir(), DisableDiscovery: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	provider := &conflictingCloudUpload{}
	if err := d.Cloud.RegisterProvider("conflict_test", provider); err != nil {
		t.Fatal(err)
	}
	cfg, err := d.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider = true, "conflict_test"
	if err := d.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	d.uploads.Add(1)
	d.runCloudUpload("not-read", "game__main__snap.zip", d.Log)
	if provider.uploads != 0 {
		t.Fatal("automatic upload overwrote a remote snapshot")
	}
	records := d.Cloud.UploadActivity()
	if len(records) != 1 || records[0].Status != "failed" || records[0].Failure != "conflict" {
		t.Fatalf("collision activity = %#v", records)
	}
}

func TestAutomaticCloudUploadFailureIsVisibleWithoutLoggingSecrets(t *testing.T) {
	d, err := New(Options{HomeOverride: t.TempDir(), DisableDiscovery: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	if err := d.Cloud.RegisterProvider("failure_test", failingCloudUpload{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := d.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled, cfg.Provider = true, "failure_test"
	if err := d.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	d.uploads.Add(1)
	d.runCloudUpload("not-read-by-provider", "game__main__snap.zip", d.Log)
	records := d.Cloud.UploadActivity()
	if len(records) != 1 || records[0].Status != "failed" || records[0].Failure != "transfer" {
		t.Fatalf("automatic upload activity = %#v", records)
	}
	for _, entry := range d.Log.History() {
		if strings.Contains(entry.Message, "secret-value") {
			t.Fatalf("provider error leaked to activity log: %q", entry.Message)
		}
	}
}
