# Windows-first UI Information Architecture

Status: Phase 2 implementation baseline.

## Intent

The default desktop experience should answer four everyday questions quickly:

1. Which games are protected?
2. Is cloud backup configured and healthy?
3. What has happened recently?
4. Where can behavior be changed?

This is an information-architecture change, not a removal of OpenSave capabilities. P2P, LAN discovery, internet relay, Linux, Steam Deck, CLI, and the existing routes remain intact.

## Primary navigation

The sidebar presents these items, in order:

| Label | Existing route | Purpose |
|---|---|---|
| Games | `home` | Scan, track, inspect, and sync the game library. |
| Cloud Backup | `cloud` | Configure the user's own cloud storage and view backup state. |
| Activity | `activity` | Review sync and backup events. |
| Settings | `settings` | Configure ordinary and advanced behavior. |

The existing `devices` and `changelog` routes are deliberately not deleted. They are reached from **Settings → Advanced → Advanced tools**. While either route is open, Settings remains highlighted so the user understands where the page belongs.

Pending pairing requests continue to surface as a badge on Settings. This prevents a navigation simplification from hiding a request that needs action.

## Home / Games page

The page is named Games in the primary experience. The translated surface includes page actions, the manual-folder form, protection statistics, empty-state guidance, library controls, game-card status, and the complete auto-scan interaction. Scan filters, grouping explanations, file metadata, exclusion confirmations, and tracking results switch language together.

## Cloud Backup and Activity

Cloud Backup now switches its complete interaction as one language surface: provider setup, OAuth guidance, custom-app credentials, cloud browsing, upload/restore/delete controls, `.sscb` import/export, progress, safety explanations, confirmations, and results. Provider names remain their official product names where appropriate. No provider behavior or credential storage changed.

Activity translates its page shell, empty state, severity labels, and time formatting. Log message bodies remain the daemon's original diagnostic text so troubleshooting details are not rewritten or obscured.

## Advanced tools

Device Sync covers the existing LAN pairing and internet-relay features. Changelog remains available for release details. These are normal buttons with keyboard focus treatment, not hidden links or new implementations.

## Visible brand and application icon

The packaged desktop product is shown as **GameSave Go** in the Wails window, custom title bar, status bar, About dialog, update messages, Windows tray title/menu, executable name, and Windows package configuration. The in-app GitHub link and release checker now point to `lyx5710317/gamesave-go`; Windows update selection prefers `GameSaveGo.exe` and retains `OpenSave.exe` as a one-way compatibility fallback across the rename.

The supplied cloud-and-controller artwork is used for the 1024 px application image, the 256 px frontend image, and a Windows ICO containing 16, 20, 24, 32, 40, 48, 64, 96, 128, and 256 px entries. The outside canvas is transparent while the supplied artwork itself is unchanged.

## Compatibility boundaries

- Route IDs, daemon endpoints, storage, snapshot behavior, P2P protocol, cloud providers, and the update transport are unchanged.
- Internal `OpenSave` and `.opensave` identifiers remain unchanged.
- Existing cloud folder names, CLI names, environment variables, database/log paths, and Linux launcher integration remain compatible.
- No SQLite migration is involved.
- The layout uses wrapping grids and existing scalable CSS primitives; the minimum desktop window remains the current Wails setting until packaging work defines a supported range.

## Native Windows validation

The packaged `GameSaveGo.exe` was launched under WebView2 with the new title and icon. Pressing the close control made the window unavailable while the process continued running, which exercises the tray-ready hide path. Launching the executable again restored the same single-instance window rather than creating a duplicate. The restored window and About dialog showed the new icon, GameSave Go identity, current version metadata, and retained upstream attribution.

## Remaining Phase 2 work

Phase 2 is complete. Packaging, signing, installer behavior, and clean-machine upgrade testing remain Phase 6 release work.
