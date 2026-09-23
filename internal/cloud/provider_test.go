package cloud

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

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
