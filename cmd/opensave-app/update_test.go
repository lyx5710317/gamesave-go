package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/selfupdate"
)

func TestDesktopProductVersion(t *testing.T) {
	if AppVersion != "1.1" {
		t.Fatalf("desktop version = %q, want 1.1", AppVersion)
	}
	if got := NewApp().AppInfo()["version"]; got != AppVersion {
		t.Fatalf("About version = %q, want %q", got, AppVersion)
	}
	for _, tc := range []struct {
		path, section, field, want string
	}{
		{"wails.json", "info", "productVersion", "1.1.0"},
		{"build/windows/info.json", "fixed", "file_version", "1.1.0.0"},
		{"build/windows/info.json", "0000", "ProductVersion", "1.1"},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(data, &metadata); err != nil {
			t.Fatal(err)
		}
		section := metadata[tc.section]
		if tc.section == "0000" {
			section = metadata["info"].(map[string]any)["0000"]
		}
		value := section.(map[string]any)[tc.field]
		if value != tc.want {
			t.Errorf("%s %s = %v, want %s", tc.path, tc.field, value, tc.want)
		}
	}
}

func TestVersionStampUsesDesktopRelease(t *testing.T) {
	home := t.TempDir()
	if previous := stampVersionFile(home); previous != "" {
		t.Fatalf("first run returned previous version %q", previous)
	}
	data, err := os.ReadFile(filepath.Join(home, "last-version"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte(AppVersion+"|")) {
		t.Fatalf("version stamp %q does not use desktop release %q", data, AppVersion)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.0.0", "2.0.0", 0},
		{"2.1.0", "2.0.0", 1},
		{"2.0.0", "2.1.0", -1},
		{"2.0.10", "2.0.9", 1}, // numeric, not lexical
		{"2.1", "2.0.5", 1},
		{"2.0", "2.0.0", 0},
		{"10.0.0", "9.9.9", 1},
		{"1.0.0", "2.0.0", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSelectUpdateAssetFor(t *testing.T) {
	assets := []releaseAsset{
		{Name: "OpenSave.Setup.exe", BrowserDownloadURL: "u/setup"},
		{Name: "OpenSave.exe", BrowserDownloadURL: "u/portable"},
		{Name: "GameSaveGo.exe", BrowserDownloadURL: "u/branded"},
		{Name: "opensave-cli.exe", BrowserDownloadURL: "u/cli"},
		{Name: "opensave-relay.exe", BrowserDownloadURL: "u/relay"},
		{Name: "opensave-linux-amd64.tar.gz", BrowserDownloadURL: "u/linux"},
	}
	if got := selectUpdateAssetFor(assets, "windows"); got != "u/branded" {
		t.Errorf("windows asset = %q, want u/branded", got)
	}
	if got := selectUpdateAssetFor(assets, "linux"); got != "u/linux" {
		t.Errorf("linux asset = %q, want u/linux", got)
	}
	if got := selectUpdateAssetFor(nil, "linux"); got != "" {
		t.Errorf("no assets should yield empty, got %q", got)
	}
	if got := selectUpdateAssetFor([]releaseAsset{{Name: "OpenSave.exe", BrowserDownloadURL: "u/legacy"}}, "windows"); got != "u/legacy" {
		t.Errorf("legacy Windows asset = %q, want u/legacy", got)
	}
}

func TestUpdateRepositoryUsesGameSaveGoFork(t *testing.T) {
	if updateRepo != "lyx5710317/gamesave-go" {
		t.Fatalf("update repository = %q, want the GameSave Go fork", updateRepo)
	}
}

func TestExtractAppBinary(t *testing.T) {
	// Build a tarball like the release: opensave-linux/{opensave,cli,relay}.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		"opensave-linux/opensave-cli":   "cli-bytes",
		"opensave-linux/opensave":       "APP-BINARY-CONTENT",
		"opensave-linux/opensave-relay": "relay-bytes",
	}
	for name, content := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(content)), Mode: 0o755})
		_, _ = tw.Write([]byte(content))
	}
	tw.Close()
	gz.Close()

	archive := filepath.Join(t.TempDir(), "rel.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o666); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "opensave.new")
	if err := selfupdate.ExtractFromTarGz(archive, "opensave", dest); err != nil {
		t.Fatalf("ExtractFromTarGz: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "APP-BINARY-CONTENT" {
		t.Errorf("extracted the wrong file: %q", got)
	}
}
