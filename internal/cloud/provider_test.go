package cloud

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

type guardedProvider struct {
	files   []CloudFile
	listErr error
	uploads int
}

func (p *guardedProvider) Upload(_, _ string) error   { p.uploads++; return nil }
func (p *guardedProvider) List() ([]CloudFile, error) { return p.files, p.listErr }
func (p *guardedProvider) Download(_, _ string) error { return nil }
func (p *guardedProvider) Delete(CloudFile) error     { return nil }

func TestUploadIfAbsentFailsClosedOnCollisionOrListingFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   []CloudFile
		listErr error
		wantErr error
		failure string
	}{
		{name: "collision", files: []CloudFile{{Name: "game__main__snap.zip", SizeBytes: 0}}, wantErr: ErrRemoteSnapshotConflict, failure: "conflict"},
		{name: "list error", listErr: errors.New("list unavailable"), failure: "transfer"},
		{name: "new snapshot", files: []CloudFile{{Name: "other.zip"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db := newTestService(t)
			provider := &guardedProvider{files: tc.files, listErr: tc.listErr}
			if err := svc.RegisterProvider("guarded_test", provider); err != nil {
				t.Fatal(err)
			}
			setCloudConfig(t, db, func(c *store.CloudConfig) { c.Enabled, c.Provider = true, "guarded_test" })
			err := svc.UploadIfAbsent("source.zip", "game__main__snap.zip")
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.listErr != nil && !errors.Is(err, tc.listErr) {
				t.Fatalf("list error = %v", err)
			}
			if tc.wantErr == nil && tc.listErr == nil && err != nil {
				t.Fatal(err)
			}
			wantUploads := 1
			if tc.failure != "" {
				wantUploads = 0
			}
			if provider.uploads != wantUploads {
				t.Fatalf("uploads = %d, want %d", provider.uploads, wantUploads)
			}
			records := svc.UploadActivity()
			if len(records) != 1 || records[0].Failure != tc.failure {
				t.Fatalf("activity = %#v", records)
			}
		})
	}
}

type recordingProvider struct {
	calls []string
}

func (p *recordingProvider) Upload(filePath, fileName string) error {
	p.calls = append(p.calls, "upload:"+filePath+":"+fileName)
	return nil
}

func (p *recordingProvider) List() ([]CloudFile, error) {
	p.calls = append(p.calls, "list")
	return []CloudFile{{Name: "remote.zip"}}, nil
}

func (p *recordingProvider) Download(fileName, localPath string) error {
	p.calls = append(p.calls, "download:"+fileName+":"+localPath)
	return nil
}

func (p *recordingProvider) Delete(file CloudFile) error {
	p.calls = append(p.calls, "delete:"+file.Name)
	return nil
}

func TestRegisteredProviderRoutesAllSnapshotOperations(t *testing.T) {
	svc, db := newTestService(t)
	provider := &recordingProvider{}
	if err := svc.RegisterProvider("test_provider", provider); err != nil {
		t.Fatal(err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "test_provider"
	})
	localPath := filepath.Join(t.TempDir(), "remote.zip")
	if err := svc.Upload("source.zip", "remote.zip"); err != nil {
		t.Fatal(err)
	}
	files, err := svc.List()
	if err != nil || len(files) != 1 || files[0].Name != "remote.zip" {
		t.Fatalf("List = %v, %v", files, err)
	}
	if err := svc.Download("remote.zip", localPath); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(files[0]); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"upload:source.zip:remote.zip", "list",
		"download:remote.zip:" + localPath, "delete:remote.zip",
	}
	if strings.Join(provider.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", provider.calls, want)
	}
}

func TestProviderRegistrationCannotReplaceExistingProvider(t *testing.T) {
	svc, _ := newTestService(t)
	for _, name := range []string{"local", "webdav", "webhook", "google_drive", "dropbox", "onedrive"} {
		if err := svc.RegisterProvider(name, &recordingProvider{}); err == nil {
			t.Errorf("replaced built-in provider %q", name)
		}
	}
	if err := svc.RegisterProvider("new_provider", &recordingProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RegisterProvider("new_provider", &recordingProvider{}); err == nil {
		t.Fatal("replaced registered provider")
	}
	for _, name := range []string{"", "UPPER", "has space", "2start"} {
		if err := svc.RegisterProvider(name, &recordingProvider{}); err == nil {
			t.Errorf("accepted invalid provider name %q", name)
		}
	}
	if err := svc.RegisterProvider("nil_provider", nil); err == nil {
		t.Fatal("accepted nil provider")
	}
}

func TestUnknownOrDisabledProviderCannotPerformOperations(t *testing.T) {
	svc, db := newTestService(t)
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = true
		c.Provider = "unknown_provider"
	})
	if _, err := svc.List(); err == nil || !strings.Contains(err.Error(), "unsupported cloud sync provider") {
		t.Fatalf("List with unknown provider: %v", err)
	}
	if err := svc.Upload("missing.zip", "remote.zip"); err == nil || !strings.Contains(err.Error(), "unsupported cloud sync provider") {
		t.Fatalf("Upload with unknown provider: %v", err)
	}
	setCloudConfig(t, db, func(c *store.CloudConfig) {
		c.Enabled = false
		c.Provider = "local"
	})
	if _, err := svc.List(); err == nil || !IsNotConfigured(err) {
		t.Fatalf("List when disabled: %v", err)
	}
	if err := svc.Delete(CloudFile{Name: "remote.zip"}); err == nil || !IsNotConfigured(err) {
		t.Fatalf("Delete when disabled: %v", err)
	}
	if err := svc.Download("remote.zip", "local.zip"); err == nil || !IsNotConfigured(err) {
		t.Fatalf("Download when disabled: %v", err)
	}
}
