package cloud

import (
	"errors"
	"fmt"
)

// ErrRemoteSnapshotConflict means a snapshot name is already present at the
// destination. Equal names or sizes are not proof of equal save contents.
var ErrRemoteSnapshotConflict = errors.New("remote snapshot name already exists; refusing to overwrite")

// ErrRemoteSnapshotAmbiguous means a provider has multiple files with the
// requested snapshot name. A name alone is not enough to select one safely.
var ErrRemoteSnapshotAmbiguous = errors.New("multiple remote snapshots share this name; refusing to choose one")

// Provider is the snapshot transport boundary. Implementations must not
// restore save data themselves: downloads remain subject to the caller's
// snapshot verification and restore safety checks.
type Provider interface {
	Upload(filePath, fileName string) error
	List() ([]CloudFile, error)
	Download(fileName, localPath string) error
	Delete(file CloudFile) error
}

// RegisterProvider adds an isolated transport without replacing an existing
// provider. Registration is safe during startup or while other cloud calls run.
func (s *Service) RegisterProvider(name string, provider Provider) error {
	if !validProviderName(name) || provider == nil {
		return fmt.Errorf("invalid cloud provider registration")
	}
	s.providersMu.Lock()
	defer s.providersMu.Unlock()
	if _, exists := s.providers[name]; exists {
		return fmt.Errorf("cloud provider %q is already registered", name)
	}
	s.providers[name] = provider
	return nil
}

func validProviderName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, ch := range name {
		if ch != '_' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func (s *Service) selectedProvider() (Provider, error) {
	_, provider, err := s.selectedProviderNamed()
	return provider, err
}

func (s *Service) selectedProviderNamed() (string, Provider, error) {
	cfg, err := s.config()
	if err != nil {
		return "", nil, err
	}
	s.providersMu.RLock()
	provider, ok := s.providers[cfg.Provider]
	s.providersMu.RUnlock()
	if !ok {
		return "", nil, fmt.Errorf("unsupported cloud sync provider: %s", cfg.Provider)
	}
	return cfg.Provider, provider, nil
}

// Upload sends a snapshot zip to the configured provider. Errors are returned
// so the snapshot hook can report upload failures without losing the snapshot.
func (s *Service) Upload(filePath, fileName string) error {
	name, provider, err := s.selectedProviderNamed()
	if err != nil {
		return err
	}
	id := s.beginUpload(name, fileName)
	err = provider.Upload(filePath, fileName)
	s.finishUpload(id, err)
	return err
}

// UploadIfAbsent is the conservative entry point for automatic and manual
// snapshot mirroring. Listing failures stop the upload; a name collision is
// not treated as an already-synced backup. This read-before-write check is
// not atomic across devices: providers must also enforce create-only writes
// before this can be a complete multi-device guarantee.
func (s *Service) UploadIfAbsent(filePath, fileName string) error {
	name, provider, err := s.selectedProviderNamed()
	if err != nil {
		return err
	}
	id := s.beginUpload(name, fileName)
	if name == "google_drive" {
		err = s.uploadDriveIfAbsent(provider, filePath, fileName)
	} else {
		err = uploadIfAbsent(provider, filePath, fileName)
	}
	s.finishUpload(id, err)
	return err
}

type driveUploadAttempt struct {
	done chan struct{}
	err  error
}

// Drive permits duplicate names. Automatic and manual backups of the same
// name in one process must share the outcome of the in-flight check and write.
// An overlapping call must not try again after an unconfirmed completion.
func (s *Service) uploadDriveIfAbsent(provider Provider, filePath, fileName string) error {
	attempt := &driveUploadAttempt{done: make(chan struct{})}
	if existing, loaded := s.driveUploads.LoadOrStore(fileName, attempt); loaded {
		previous := existing.(*driveUploadAttempt)
		<-previous.done
		if previous.err == nil {
			return ErrRemoteSnapshotConflict
		}
		return previous.err
	}
	defer s.driveUploads.Delete(fileName)
	defer close(attempt.done)
	attempt.err = uploadIfAbsent(provider, filePath, fileName)
	return attempt.err
}

func uploadIfAbsent(provider Provider, filePath, fileName string) error {
	files, err := provider.List()
	if err == nil {
		for _, file := range files {
			if file.Name == fileName {
				err = ErrRemoteSnapshotConflict
				break
			}
		}
	}
	if err == nil {
		err = provider.Upload(filePath, fileName)
	}
	return err
}

// List returns the configured provider's snapshot zips.
func (s *Service) List() ([]CloudFile, error) {
	provider, err := s.selectedProvider()
	if err != nil {
		return nil, err
	}
	return provider.List()
}

// Download fetches a remote snapshot to localPath. The caller is responsible
// for verification before any restore or incoming replacement.
func (s *Service) Download(fileName, localPath string) error {
	provider, err := s.selectedProvider()
	if err != nil {
		return err
	}
	return provider.Download(fileName, localPath)
}

// Delete removes one remote snapshot only when explicitly requested by the
// caller. Providers that cannot delete must return an error.
func (s *Service) Delete(file CloudFile) error {
	provider, err := s.selectedProvider()
	if err != nil {
		return err
	}
	return provider.Delete(file)
}

// legacyProvider keeps existing provider behavior intact as the providers
// are migrated one at a time. It is not a second storage implementation.
type legacyProvider struct{ service *Service }

func (p legacyProvider) Upload(filePath, fileName string) error {
	return p.service.uploadLegacy(filePath, fileName)
}

func (p legacyProvider) List() ([]CloudFile, error) { return p.service.listLegacy() }

func (p legacyProvider) Download(fileName, localPath string) error {
	return p.service.downloadLegacy(fileName, localPath)
}

func (p legacyProvider) Delete(file CloudFile) error { return p.service.deleteLegacy(file) }

var _ Provider = legacyProvider{}
