package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/opensave/opensave/internal/selfupdate"
	"gopkg.in/yaml.v3"
)

func TestDesktopProductVersion(t *testing.T) {
	if AppVersion != "1.1.1" {
		t.Fatalf("desktop version = %q, want 1.1.1", AppVersion)
	}
	if got := NewApp().AppInfo()["version"]; got != AppVersion {
		t.Fatalf("About version = %q, want %q", got, AppVersion)
	}
	for _, tc := range []struct {
		path, section, field, want string
	}{
		{"frontend/package.json", "", "version", "1.1.1"},
		{"frontend/package-lock.json", "", "version", "1.1.1"},
		{"wails.json", "info", "productVersion", "1.1.1"},
		{"build/windows/info.json", "fixed", "file_version", "1.1.1.0"},
		{"build/windows/info.json", "0000", "ProductVersion", "1.1.1"},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(data, &metadata); err != nil {
			t.Fatal(err)
		}
		var section any = metadata
		if tc.section == "0000" {
			section = metadata["info"].(map[string]any)["0000"]
		} else if tc.section != "" {
			section = metadata[tc.section]
		}
		value := section.(map[string]any)[tc.field]
		if value != tc.want {
			t.Errorf("%s %s = %v, want %s", tc.path, tc.field, value, tc.want)
		}
	}
}

func TestDesktopReleaseTagLinkerStamp(t *testing.T) {
	if want := os.Getenv("EXPECTED_DESKTOP_RELEASE_TAG"); want != "" && DesktopReleaseTag != want {
		t.Fatalf("release tag stamp = %q, want %q", DesktopReleaseTag, want)
	}
}

func TestShouldOfferDesktopRelease(t *testing.T) {
	tests := []struct {
		name, releaseTag, currentVersion, installedTag string
		want                                           bool
	}{
		{"legacy 1.1 development build", "v1.1.1", "1.1", "", true},
		{"current development build", "v1.1.1", "1.1.1", "", true},
		{"matching installed release", "v1.1.1", "1.1.1", "v1.1.1", false},
		{"newer release", "v1.1.2", "1.1.1", "v1.1.1", true},
		{"older release", "v1.1.0", "1.1.1", "", false},
		{"unexpected equal-version tag", "v1.1.1+other", "1.1.1", "", false},
		{"empty release", "", "1.1.1", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldOfferDesktopRelease(selfupdate.Release{TagName: tc.releaseTag}, tc.currentVersion, tc.installedTag)
			if got != tc.want {
				t.Errorf("shouldOfferDesktopRelease(%q, %q, %q) = %t, want %t", tc.releaseTag, tc.currentVersion, tc.installedTag, got, tc.want)
			}
		})
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
		{"1.1.1", "1.1", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSelectUpdateAssetFor(t *testing.T) {
	assets := []releaseAsset{
		{Name: "opensave-linux-arm64.tar.gz", BrowserDownloadURL: "u/arm-cli-only"},
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
	if got := selectUpdateAssetForArch(assets, "linux", "arm64"); got != "" {
		t.Errorf("arm64 CLI-only archive must not be offered as a desktop app: %q", got)
	}
	if got := selectUpdateAssetFor([]releaseAsset{{Name: "OpenSave.exe", BrowserDownloadURL: "u/legacy"}}, "windows"); got != "u/legacy" {
		t.Errorf("legacy Windows asset = %q, want u/legacy", got)
	}
}

func desktopReleaseAsset(name string) releaseAsset {
	return releaseAsset{
		Name: name, Size: 42,
		BrowserDownloadURL: "https://github.com/" + updateRepo + "/releases/download/v1.1.1/" + name,
	}
}

func TestReleaseUpdateAssetsRequireSameReleaseAndChecksum(t *testing.T) {
	portable := desktopReleaseAsset("GameSaveGo.exe")
	manifest := desktopReleaseAsset("SHA256SUMS")
	installer := desktopReleaseAsset("GameSaveGo.Setup.exe")
	rel := selfupdate.Release{TagName: "v1.1.1", Assets: []releaseAsset{portable, manifest, installer}}
	got, sums, err := releaseUpdateAssets(rel, portable.BrowserDownloadURL, "windows")
	if err != nil || got != portable || sums != manifest {
		t.Fatalf("valid update plan = %+v, %+v, %v", got, sums, err)
	}
	if _, err := selectInstallerAsset(rel); err != nil {
		t.Fatalf("matching installer rejected: %v", err)
	}
	for _, tc := range []struct {
		name, url string
		rel       selfupdate.Release
	}{
		{"other website", "https://example.com/evil.exe", rel},
		{"installer in place of portable", installer.BrowserDownloadURL, rel},
		{"missing manifest", portable.BrowserDownloadURL, selfupdate.Release{TagName: rel.TagName, Assets: []releaseAsset{portable}}},
		{"duplicate manifest", portable.BrowserDownloadURL, selfupdate.Release{TagName: rel.TagName, Assets: []releaseAsset{portable, manifest, manifest}}},
		{"different release tag", portable.BrowserDownloadURL, selfupdate.Release{TagName: "v1.1.2", Assets: rel.Assets}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := releaseUpdateAssets(tc.rel, tc.url, "windows"); err == nil {
				t.Fatal("untrusted update was accepted")
			}
		})
	}
}

func TestInstallerCommandDoesNotInterpolatePath(t *testing.T) {
	path := `C:\Users\O'Brien\AppData\Local\Temp\GameSaveGo.Setup.exe`
	cmd := installerCommand(path)
	if strings.Contains(strings.Join(cmd.Args, " "), path) {
		t.Fatal("installer path was interpolated into PowerShell source")
	}
	want := "GAMESAVE_UPDATE_INSTALLER=" + path
	if !slices.Contains(cmd.Env, want) {
		t.Fatalf("installer path is not passed as an environment value")
	}
}

func TestUpdateRepositoryUsesGameSaveGoFork(t *testing.T) {
	if updateRepo != "lyx5710317/gamesave-go" {
		t.Fatalf("update repository = %q, want the GameSave Go fork", updateRepo)
	}
}

func TestReleaseWorkflowSeparatesDesktopAndPeerVersions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On struct {
			WorkflowDispatch struct {
				Inputs map[string]struct {
					Required bool   `yaml:"required"`
					Type     string `yaml:"type"`
				} `yaml:"inputs"`
			} `yaml:"workflow_dispatch"`
		} `yaml:"on"`
		Jobs map[string]struct {
			If          string            `yaml:"if"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Name string            `yaml:"name"`
				If   string            `yaml:"if"`
				Env  map[string]string `yaml:"env"`
				Run  string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	candidate, ok := workflow.On.WorkflowDispatch.Inputs["candidate_version"]
	if !ok || !candidate.Required || candidate.Type != "string" {
		t.Fatal("manual candidate builds must require an expected desktop version")
	}
	preflight, ok := workflow.Jobs["preflight"]
	if !ok {
		t.Fatal("missing release preflight")
	}
	if len(preflight.Steps) < 2 ||
		!strings.Contains(preflight.Steps[1].Env["EXPECTED_TAG"], "inputs.candidate_version") ||
		!strings.Contains(preflight.Steps[1].Run, "refs/tags/") {
		t.Fatal("candidate preflight does not validate the requested version on a branch")
	}
	for _, jobName := range []string{"windows", "linux"} {
		job, ok := workflow.Jobs[jobName]
		if !ok {
			t.Fatalf("missing %s release build", jobName)
		}
		var buildScripts strings.Builder
		for _, step := range job.Steps {
			buildScripts.WriteString(step.Run)
		}
		build := buildScripts.String()
		if strings.Contains(build, "internal/version.Version=") {
			t.Errorf("%s release build stamps desktop tag onto inherited peer version", jobName)
		}
		if !strings.Contains(build, "internal/version.BuildTime=") {
			t.Errorf("%s release build omits peer build timestamp", jobName)
		}
		if !strings.Contains(build, "-X github.com/opensave/opensave/cmd/opensave-app.DesktopReleaseTag=${GITHUB_REF_NAME}") {
			t.Errorf("%s release build omits desktop release identity", jobName)
		}
	}
	windows := workflow.Jobs["windows"]
	var candidateBuild, signingStep string
	for _, step := range windows.Steps {
		switch step.Name {
		case "Build Windows app (NSIS installer + portable exe)":
			candidateBuild = step.Run
		case "Code sign (optional)":
			signingStep = step.If
		}
	}
	if !strings.Contains(candidateBuild, `if [ "$GITHUB_EVENT_NAME" = "push" ]; then`) ||
		!strings.Contains(candidateBuild, `wails build -nsis -ldflags "$LD"`) ||
		!strings.Contains(signingStep, "github.event_name == 'push'") {
		t.Fatal("manual candidate build must remain unsigned and avoid stamping a branch as a release")
	}
	if !strings.Contains(workflow.Jobs["linux"].If, "github.event_name == 'push'") ||
		!strings.Contains(workflow.Jobs["relay-docker"].If, "github.event_name == 'push'") {
		t.Fatal("manual Windows candidates must not start other release builds")
	}
	release, ok := workflow.Jobs["release"]
	if !ok {
		t.Fatal("missing release job")
	}
	if !strings.Contains(release.If, "github.event_name == 'push'") ||
		!strings.Contains(release.If, "GAMESAVE_GO_PUBLIC_RELEASE_READY") ||
		release.Permissions["contents"] != "write" {
		t.Fatal("public release is not behind its explicit write-permission gate")
	}
	var releaseScripts strings.Builder
	for _, step := range release.Steps {
		releaseScripts.WriteString(step.Run)
	}
	if !strings.Contains(releaseScripts.String(), "HAS_SIGNING") {
		t.Fatal("public release does not require Windows signing credentials")
	}
	if !strings.Contains(string(raw), "GameSaveGo.Setup.exe") || strings.Contains(string(raw), "discord.gg") {
		t.Fatal("release still uses the upstream installer name or community invitation")
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
