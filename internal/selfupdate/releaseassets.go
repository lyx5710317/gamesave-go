package selfupdate

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxReleaseChecksums  = 1 << 20
	maxReleaseAssetBytes = 1 << 30
)

// FindReleaseAsset accepts only a uniquely named asset at the canonical
// GitHub download URL for this exact repository and release tag. A URL
// supplied by the webview is never itself authority to run a binary.
func FindReleaseAsset(rel Release, repo, name string) (Asset, error) {
	if !safeReleaseComponent(rel.TagName) || !safeReleaseComponent(name) || !safeReleaseRepo(repo) {
		return Asset{}, fmt.Errorf("invalid release identity")
	}
	wantURL := "https://github.com/" + repo + "/releases/download/" + rel.TagName + "/" + name
	var found Asset
	count := 0
	for _, asset := range rel.Assets {
		if asset.Name != name {
			continue
		}
		count++
		found = asset
	}
	if count != 1 || found.BrowserDownloadURL != wantURL || found.Size <= 0 || found.Size > maxReleaseAssetBytes {
		return Asset{}, fmt.Errorf("release asset %q is missing, duplicated, or has an unexpected download location", name)
	}
	return found, nil
}

func safeReleaseComponent(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func safeReleaseRepo(repo string) bool {
	parts := strings.Split(repo, "/")
	return len(parts) == 2 && safeReleaseComponent(parts[0]) && safeReleaseComponent(parts[1])
}

// FetchReleaseChecksum reads the bounded SHA256SUMS for a release asset.
// The caller must first validate the manifest URL with FindReleaseAsset.
// Redirects are limited to HTTPS GitHub-owned asset hosts.
func FetchReleaseChecksum(manifestURL, assetName string) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Port() != "" ||
				(req.URL.Hostname() != "github.com" && !strings.HasSuffix(req.URL.Hostname(), ".githubusercontent.com")) {
				return fmt.Errorf("release checksum redirected outside trusted GitHub hosts")
			}
			return nil
		},
	}
	resp, err := client.Get(manifestURL)
	if err != nil {
		return empty, fmt.Errorf("fetch release checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return empty, fmt.Errorf("release checksums returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseChecksums+1))
	if err != nil {
		return empty, fmt.Errorf("read release checksums: %w", err)
	}
	if len(data) > maxReleaseChecksums {
		return empty, fmt.Errorf("release checksum list is too large")
	}
	return ParseReleaseChecksum(data, assetName)
}

// ParseReleaseChecksum refuses absent, duplicate, or malformed entries.
func ParseReleaseChecksum(data []byte, assetName string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	if !safeReleaseComponent(assetName) {
		return digest, fmt.Errorf("invalid release asset name")
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		count++
		if len(fields[0]) != sha256.Size*2 {
			return digest, fmt.Errorf("invalid checksum for %s", assetName)
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil {
			return digest, fmt.Errorf("invalid checksum for %s", assetName)
		}
		copy(digest[:], decoded)
	}
	if count != 1 {
		return digest, fmt.Errorf("release checksum for %s is missing or duplicated", assetName)
	}
	return digest, nil
}

// DownloadVerified checks GitHub's declared size and the release manifest's
// SHA-256 before the caller extracts, swaps, or launches the downloaded file.
// On every failure the candidate is removed.
func DownloadVerified(url, path string, expectedSize int64, expectedHash [sha256.Size]byte, progress func(done, total int64)) error {
	if expectedSize <= 0 || expectedSize > maxReleaseAssetBytes {
		return fmt.Errorf("release asset has an invalid declared size")
	}
	if err := downloadWithMaxBytes(url, path, expectedSize, progress); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("download release asset: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != expectedSize {
		_ = os.Remove(path)
		return fmt.Errorf("downloaded release asset size differs from published size")
	}
	f, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	actual := sha256.New()
	_, copyErr := io.Copy(actual, f)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("could not read downloaded release asset")
	}
	if subtle.ConstantTimeCompare(actual.Sum(nil), expectedHash[:]) != 1 {
		_ = os.Remove(path)
		return fmt.Errorf("downloaded release asset checksum does not match SHA256SUMS")
	}
	return nil
}
