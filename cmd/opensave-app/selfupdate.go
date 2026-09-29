package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/opensave/opensave/internal/selfupdate"
	"github.com/opensave/opensave/internal/version"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Self-update: replace the running executable with a newer build and
// restart. Windows can't overwrite a running exe, but it CAN be renamed —
// so the dance is: rename running exe aside (.old), move the downloaded
// build into its place, spawn it, quit. The new process deletes the .old
// leftover once this one has exited (see cleanupReplacedBinary).

// updateEvent broadcasts install progress to the dashboard so the UI can
// show a live banner ("downloading 42%…") instead of appearing to hang.
func (a *App) updateEvent(state string, pct int, errMsg string) {
	if a.server == nil {
		return
	}
	a.server.Hub.Broadcast("app-update", map[string]any{
		"state": state, "percentage": pct, "error": errMsg,
	})
}

// runningInFlatpak reports whether the app runs inside a Flatpak sandbox,
// where /app is read-only and the rename-swap self-update cannot work —
// updates arrive through flatpak itself (or a newer bundle from the
// release page).
func runningInFlatpak() bool {
	if os.Getenv("FLATPAK_ID") != "" {
		return true
	}
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}

const flatpakUpdateMsg = productName + " is installed through Flatpak; use your package source's Flatpak update mechanism."

// InstallUpdateFromPeer remains as a binding for older frontend assets, but
// peer announcements carry the inherited sync-component version, not a
// verified GameSave Go desktop release identity. Never install that binary
// as a product update. P2P save synchronization itself remains available.
func (a *App) InstallUpdateFromPeer(_ string) string {
	return "Device binary updates are unavailable; install a verified GameSave Go release from this project's GitHub page."
}

// InstallUpdateFromURL downloads a release asset (the .exe from a GitHub
// release) and installs it. Same async contract as InstallUpdateFromPeer.
//
// Installs living in a protected folder (Program Files) can't be swapped
// by an unelevated rename — for those, the NSIS installer is downloaded
// and launched instead; it requests UAC elevation itself.
func (a *App) InstallUpdateFromURL(url string) string {
	if !strings.HasPrefix(url, "https://github.com/"+updateRepo+"/releases/download/") {
		return "update URL must belong to a GameSave Go release"
	}
	if runningInFlatpak() {
		return flatpakUpdateMsg
	}
	go func() {
		rel, err := selfupdate.LatestRelease(updateRepo, "GameSaveGo/"+AppVersion, a.wantsPreReleases())
		if err != nil || !shouldOfferDesktopRelease(rel, AppVersion, DesktopReleaseTag) {
			a.updateEvent("error", 0, "could not verify the offered release; refresh the update check")
			return
		}
		asset, sums, err := releaseUpdateAssets(rel, url, runtime.GOOS)
		if err != nil {
			a.updateEvent("error", 0, "update asset is not verified for this release")
			return
		}
		expected, err := selfupdate.FetchReleaseChecksum(sums.BrowserDownloadURL, asset.Name)
		if err != nil {
			a.updateEvent("error", 0, "could not verify release checksum: "+err.Error())
			return
		}
		exe, err := os.Executable()
		if err != nil {
			a.updateEvent("error", 0, err.Error())
			return
		}
		// Windows Program Files installs can't be swapped unelevated — run
		// the NSIS installer (with its UAC prompt) instead.
		if runtime.GOOS == "windows" && !selfupdate.CanStageUpdate(exe) {
			a.installViaInstaller(rel, sums)
			return
		}

		a.updateEvent("downloading", 0, "")
		progress := func(done, total int64) {
			if total > 0 {
				a.updateEvent("downloading", int(done*100/total), "")
			}
		}

		newBinary := exe + ".new"
		defer os.Remove(newBinary)

		if strings.HasSuffix(strings.ToLower(asset.Name), ".tar.gz") || strings.HasSuffix(strings.ToLower(asset.Name), ".tgz") {
			// Linux ships a tarball: download it, extract the app binary.
			archive, err := os.CreateTemp("", "opensave-update-*.tar.gz")
			if err != nil {
				a.updateEvent("error", 0, err.Error())
				return
			}
			archivePath := archive.Name()
			archive.Close()
			defer os.Remove(archivePath)

			if err := selfupdate.DownloadVerified(asset.BrowserDownloadURL, archivePath, asset.Size, expected, progress); err != nil {
				a.updateEvent("error", 0, "download failed: "+err.Error())
				return
			}
			if err := selfupdate.ExtractFromTarGz(archivePath, "opensave", newBinary); err != nil {
				a.updateEvent("error", 0, "unpack failed: "+err.Error())
				return
			}
		} else {
			// Windows portable exe: download straight to the swap file.
			if err := selfupdate.DownloadVerified(asset.BrowserDownloadURL, newBinary, asset.Size, expected, progress); err != nil {
				// The probe above said this directory was writable and it
				// wasn't. Controlled Folder Access, an antivirus rule, or an
				// ACL that allows creating a randomly-named temp file but not
				// an .exe will all do that. The installer elevates and does
				// not care, so take that route rather than handing the user a
				// raw "Access is denied" they can do nothing with.
				if runtime.GOOS == "windows" && errors.Is(err, fs.ErrPermission) {
					a.installViaInstaller(rel, sums)
					return
				}
				a.updateEvent("error", 0, "download failed: "+err.Error())
				return
			}
		}
		a.finishInstall(newBinary)
	}()
	return ""
}

// installViaInstaller fetches the latest release's NSIS installer into the
// temp dir and launches it, then quits so the installer can replace the
// app's files. Launching goes through ShellExecute (Start-Process) so the
// installer's elevation request produces a UAC prompt instead of failing.
func (a *App) installViaInstaller(rel selfupdate.Release, sums selfupdate.Asset) {
	a.updateEvent("downloading", 0, "")

	inst, err := selectInstallerAsset(rel)
	if err != nil {
		a.updateEvent("error", 0, "couldn't locate a verified installer in this release: "+err.Error())
		return
	}
	expected, err := selfupdate.FetchReleaseChecksum(sums.BrowserDownloadURL, inst.Name)
	if err != nil {
		a.updateEvent("error", 0, "could not verify installer checksum: "+err.Error())
		return
	}

	tmp, err := os.CreateTemp("", "GameSaveGo.Setup-*.exe")
	if err != nil {
		a.updateEvent("error", 0, err.Error())
		return
	}
	dest := tmp.Name()
	tmp.Close()

	if err := selfupdate.DownloadVerified(inst.BrowserDownloadURL, dest, inst.Size, expected, func(done, total int64) {
		if total > 0 {
			a.updateEvent("downloading", int(done*100/total), "")
		}
	}); err != nil {
		os.Remove(dest)
		a.updateEvent("error", 0, "download failed: "+err.Error())
		return
	}
	if err := selfupdate.ValidateExecutable(dest); err != nil {
		os.Remove(dest)
		a.updateEvent("error", 0, err.Error())
		return
	}

	a.updateEvent("installing", 100, "")
	cmd := installerCommand(dest)
	if err := cmd.Start(); err != nil {
		os.Remove(dest)
		a.updateEvent("error", 0, "launch installer: "+err.Error())
		return
	}

	if a.daemon != nil {
		a.daemon.Log.Log("info", "update installer launched; quitting so it can replace the app")
	}
	a.updateEvent("restarting", 100, "")
	a.reallyQuit = true
	wailsruntime.Quit(a.ctx)
}

// installerCommand passes the verified temporary path through the process
// environment, not through interpolated PowerShell source. A username or
// temp path containing an apostrophe cannot change the command to execute.
func installerCommand(path string) *exec.Cmd {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"Start-Process -FilePath $env:GAMESAVE_UPDATE_INSTALLER")
	cmd.Env = append(os.Environ(), "GAMESAVE_UPDATE_INSTALLER="+path)
	return cmd
}

// selectInstallerAsset keeps the elevated path on the same verified release
// as the portable update. Legacy naming is allowed only for an asset listed
// by this fork's release with a matching SHA256SUMS entry.
func selectInstallerAsset(rel selfupdate.Release) (selfupdate.Asset, error) {
	for _, name := range []string{"GameSaveGo.Setup.exe", "OpenSave.Setup.exe"} {
		if asset, err := selfupdate.FindReleaseAsset(rel, updateRepo, name); err == nil {
			return asset, nil
		}
	}
	return selfupdate.Asset{}, fmt.Errorf("no verified installer asset on release %s", rel.TagName)
}

// finishInstall validates and applies a downloaded build, then restarts.
func (a *App) finishInstall(newExePath string) {
	a.updateEvent("installing", 100, "")
	if err := a.applyUpdate(newExePath); err != nil {
		a.updateEvent("error", 0, "install failed: "+err.Error())
		return
	}
	// applyUpdate quits the app on success; this line is never reached.
}

// applyUpdate swaps the running executable for newExePath and restarts.
func (a *App) applyUpdate(newExePath string) error {
	// exe is the path Swap actually replaced — resolved through any symlink,
	// so relaunch and rollback below act on the same file it moved.
	exe, old, err := selfupdate.Swap(newExePath)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe)
	// The child waits for this .old file to become deletable — i.e. for
	// this process to fully exit — before claiming the single-instance
	// lock. Without the wait, the new instance would see us still running,
	// signal us to show our window, and exit.
	cmd.Env = append(os.Environ(), "OPENSAVE_CLEANUP_OLD="+old)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(exe)
		_ = os.Rename(old, exe)
		return fmt.Errorf("launch new build: %w", err)
	}

	if a.daemon != nil {
		a.daemon.Log.Log("info", "update installed; restarting")
	}
	a.updateEvent("restarting", 100, "")
	a.reallyQuit = true
	wailsruntime.Quit(a.ctx)
	return nil
}

// stampVersionFile records the running build's identity in the data dir
// and returns the previously recorded version when it differs — i.e. this
// is the first run after an update. Returns "" on the very first run or
// when the build hasn't changed.
func stampVersionFile(homeDir string) (updatedFrom string) {
	path := filepath.Join(homeDir, "last-version")
	stamp := AppVersion + "|" + strconv.FormatInt(version.BuildTimeMs(), 10)

	prev := ""
	if raw, err := os.ReadFile(path); err == nil {
		prev = strings.TrimSpace(string(raw))
	}
	if prev == stamp {
		return ""
	}
	_ = os.WriteFile(path, []byte(stamp), 0o666)
	if prev == "" {
		return "" // first run ever — nothing to announce
	}
	if i := strings.IndexByte(prev, '|'); i >= 0 {
		prev = prev[:i]
	}
	return prev
}

// cleanupReplacedBinary runs at startup. After an update, the previous
// build lives at <exe>.old and may still be exiting; deleting it doubles
// as the "old process has fully quit" gate before Wails takes the
// single-instance lock.
func cleanupReplacedBinary() {
	if old := os.Getenv("OPENSAVE_CLEANUP_OLD"); old != "" {
		selfupdate.CleanupOld(old)
		return
	}
	// Normal start: sweep any leftover from a previous update.
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}
