package cloud

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

type activityProvider struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (p *activityProvider) Upload(_, _ string) error {
	if p.started != nil {
		p.started <- struct{}{}
		<-p.release
	}
	return p.err
}
func (p *activityProvider) List() ([]CloudFile, error) { return nil, nil }
func (p *activityProvider) Download(_, _ string) error { return nil }
func (p *activityProvider) Delete(CloudFile) error     { return nil }

func testActivityService(t *testing.T, provider Provider) *Service {
	t.Helper()
	svc, db := newTestService(t)
	if err := svc.RegisterProvider("activity_test", provider); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "activity_test"
	})
	return svc
}

func TestUploadActivityTracksRunningAndCompletion(t *testing.T) {
	provider := &activityProvider{started: make(chan struct{}, 1), release: make(chan struct{})}
	svc := testActivityService(t, provider)
	done := make(chan error, 1)
	go func() { done <- svc.Upload("C:/private/save.zip", "game__main__snap.zip") }()
	<-provider.started
	running := svc.UploadActivity()
	if len(running) != 1 || running[0].Status != "running" || running[0].GameID != "game" ||
		running[0].SnapshotID != "snap" || running[0].FinishedAt != "" {
		t.Fatalf("running activity = %#v", running)
	}
	close(provider.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	finished := svc.UploadActivity()
	if len(finished) != 1 || finished[0].Status != "succeeded" || finished[0].FinishedAt == "" {
		t.Fatalf("finished activity = %#v", finished)
	}
	// Consumers cannot modify the service's retained records.
	finished[0].Status = "failed"
	if svc.UploadActivity()[0].Status != "succeeded" {
		t.Fatal("activity snapshot aliases service state")
	}
}

func TestUploadActivityRedactsProviderErrorsAndBoundsHistory(t *testing.T) {
	secret := "access_token=super-secret"
	provider := &activityProvider{err: errors.New("request failed: " + secret)}
	svc := testActivityService(t, provider)
	for i := 0; i < maxFinishedUploads+3; i++ {
		if err := svc.Upload("C:/private/save.zip", "game__main__snap.zip"); err == nil {
			t.Fatal("provider failure was hidden")
		}
	}
	records := svc.UploadActivity()
	if len(records) != maxFinishedUploads || records[0].Status != "failed" || records[0].Failure != "transfer" ||
		records[0].ID <= records[len(records)-1].ID {
		t.Fatalf("bounded activity = %#v", records)
	}
	raw, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "private") {
		t.Fatalf("activity leaked secret or local path: %s", raw)
	}
}

func TestUploadActivityClassifiesMissingAuthWithoutRawError(t *testing.T) {
	svc := testActivityService(t, &activityProvider{err: errors.New("provider not authenticated: bearer secret")})
	if err := svc.Upload("ignored", "game__main__snap.zip"); err == nil {
		t.Fatal("provider failure was hidden")
	}
	if record := svc.UploadActivity()[0]; record.Status != "failed" || record.Failure != "configuration" {
		t.Fatalf("configuration failure = %#v", record)
	}
}
