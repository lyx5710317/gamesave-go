package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opensave/opensave/internal/store"
)

// The legacy transport exposes only a read capability for the current
// Jianguoyun preset and local-folder destination. It never creates folders or
// implements ReplaceVaultMetadata, even if a server supplies an ETag.
func (p legacyProvider) ReadVaultMetadata(ctx context.Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	cfg, err := p.service.config()
	if err != nil {
		return nil, "", err
	}
	switch cfg.Provider {
	case "jianguoyun":
		return p.service.readJianguoyunVaultMetadata(ctx, cfg)
	case "local":
		if cfg.URL == "" {
			return nil, "", errors.New("no local folder destination configured")
		}
		file, err := os.Open(filepath.Join(cfg.URL, "vault.json"))
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrVaultMetadataNotFound
		}
		if err != nil {
			return nil, "", err
		}
		defer file.Close()
		data, err := readBoundedVaultMetadata(file)
		return data, "", err
	default:
		return nil, "", ErrVaultMetadataReadUnsupported
	}
}

func (s *Service) readJianguoyunVaultMetadata(ctx context.Context, cfg store.CloudConfig) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(cfg.URL, "vault.json"), nil)
	if err != nil {
		return nil, "", err
	}
	applyBasicAuth(req, cfg.Username, cfg.Password)
	resp, err := s.jianguoyunDo(req, true)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrVaultMetadataNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", jianguoyunStatusError("检查存档库", resp.StatusCode)
	}
	data, err := readBoundedVaultMetadata(resp.Body)
	if err != nil {
		return nil, "", err
	}
	// An observed ETag is deliberately discarded: this transport has no
	// provider-proven conditional replacement contract for vault.json.
	return data, "", nil
}

func readBoundedVaultMetadata(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxVaultMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read remote vault metadata: %w", err)
	}
	if len(data) > maxVaultMetadataBytes {
		return nil, ErrVaultMetadataInvalidSize
	}
	return data, nil
}
