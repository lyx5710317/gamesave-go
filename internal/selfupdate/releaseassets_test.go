package selfupdate

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testReleaseRepo = "owner/gamesave-go"

func testReleaseAsset(name string, size int64) Asset {
	return Asset{
		Name: name, Size: size,
		BrowserDownloadURL: "https://github.com/" + testReleaseRepo + "/releases/download/v1.1.1/" + name,
	}
}

func TestFindReleaseAssetRejectsUntrustedOrAmbiguousMetadata(t *testing.T) {
	good := testReleaseAsset("GameSaveGo.exe", 42)
	rel := Release{TagName: "v1.1.1", Assets: []Asset{good}}
	if got, err := FindReleaseAsset(rel, testReleaseRepo, good.Name); err != nil || got != good {
		t.Fatalf("valid release asset rejected: %+v, %v", got, err)
	}
	for _, tc := range []struct {
		name string
		rel  Release
	}{
		{"missing", Release{TagName: "v1.1.1"}},
		{"duplicate", Release{TagName: "v1.1.1", Assets: []Asset{good, good}}},
		{"other host", Release{TagName: "v1.1.1", Assets: []Asset{{Name: good.Name, BrowserDownloadURL: "https://github.com.evil.example/release.exe", Size: 42}}}},
		{"other tag", Release{TagName: "v1.1.2", Assets: []Asset{good}}},
		{"query", Release{TagName: "v1.1.1", Assets: []Asset{{Name: good.Name, BrowserDownloadURL: good.BrowserDownloadURL + "?x=1", Size: 42}}}},
		{"zero size", Release{TagName: "v1.1.1", Assets: []Asset{{Name: good.Name, BrowserDownloadURL: good.BrowserDownloadURL}}}},
		{"oversized", Release{TagName: "v1.1.1", Assets: []Asset{testReleaseAsset(good.Name, maxReleaseAssetBytes+1)}}},
		{"unsafe tag", Release{TagName: "../v1.1.1", Assets: []Asset{good}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FindReleaseAsset(tc.rel, testReleaseRepo, good.Name); err == nil {
				t.Fatal("unsafe release asset was accepted")
			}
		})
	}
}

func TestParseReleaseChecksum(t *testing.T) {
	payload := []byte("release binary")
	want := sha256.Sum256(payload)
	line := fmt.Sprintf("%x  GameSaveGo.exe\n", want)
	got, err := ParseReleaseChecksum([]byte(line), "GameSaveGo.exe")
	if err != nil || got != want {
		t.Fatalf("valid checksum = %x, %v", got, err)
	}
	for _, tc := range []struct{ name, body string }{
		{"missing", fmt.Sprintf("%x  Other.exe\n", want)},
		{"duplicate", line + line},
		{"bad hex", strings.Repeat("z", 64) + "  GameSaveGo.exe\n"},
		{"short hash", "abc  GameSaveGo.exe\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseReleaseChecksum([]byte(tc.body), "GameSaveGo.exe"); err == nil {
				t.Fatal("bad release checksum was accepted")
			}
		})
	}
}

func TestFetchReleaseChecksumLimitsResponseAndRedirects(t *testing.T) {
	good := sha256.Sum256([]byte("a"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			fmt.Fprintf(w, "%x  GameSaveGo.exe\n", good)
		case "/large":
			fmt.Fprint(w, strings.Repeat("a", maxReleaseChecksums+1))
		case "/redirect":
			http.Redirect(w, r, "http://untrusted.example/SHA256SUMS", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	if got, err := FetchReleaseChecksum(srv.URL+"/good", "GameSaveGo.exe"); err != nil || got != good {
		t.Fatalf("checksum fetch = %x, %v", got, err)
	}
	for _, path := range []string{"/large", "/redirect", "/missing"} {
		if _, err := FetchReleaseChecksum(srv.URL+path, "GameSaveGo.exe"); err == nil {
			t.Errorf("%s did not fail closed", path)
		}
	}
}

func TestDownloadVerifiedKeepsOnlyMatchingAsset(t *testing.T) {
	payload := fakeBinary()
	want := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/interrupted" {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload[:len(payload)/2])
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	for _, tc := range []struct {
		name string
		url  string
		size int64
		hash [sha256.Size]byte
		ok   bool
	}{
		{"matching", srv.URL + "/asset", int64(len(payload)), want, true},
		{"wrong size", srv.URL + "/asset", int64(len(payload) - 1), want, false},
		{"wrong hash", srv.URL + "/asset", int64(len(payload)), sha256.Sum256([]byte("other")), false},
		{"download error", srv.URL + "/missing", int64(len(payload)), want, false},
		{"interrupted", srv.URL + "/interrupted", int64(len(payload)), want, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "candidate.exe")
			err := DownloadVerified(tc.url, path, tc.size, tc.hash, nil)
			if tc.ok {
				if err != nil {
					t.Fatal(err)
				}
				raw, readErr := os.ReadFile(path)
				if readErr != nil || sha256.Sum256(raw) != want {
					t.Fatalf("verified file missing or altered: %v", readErr)
				}
			} else {
				if err == nil {
					t.Fatal("bad asset was accepted")
				}
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("bad candidate was left behind: %v", statErr)
				}
			}
		})
	}
}
