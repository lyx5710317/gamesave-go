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

## Advanced tools

Device Sync covers the existing LAN pairing and internet-relay features. Changelog remains available for release details. These are normal buttons with keyboard focus treatment, not hidden links or new implementations.

## Compatibility boundaries

- Route IDs, daemon endpoints, storage, snapshot behavior, P2P protocol, cloud providers, and update protocol are unchanged.
- Internal `OpenSave` and `.opensave` identifiers remain unchanged.
- No SQLite migration is involved.
- The layout uses wrapping grids and existing scalable CSS primitives; the minimum desktop window remains the current Wails setting until packaging work defines a supported range.

## Remaining Phase 2 work

- Translate the Cloud Backup and Activity pages in coherent page-sized slices.
- Review visible GameSave Go branding separately from internal compatibility identifiers and update/release endpoints.
- Complete native tray minimize/restore testing on the packaged app; keyboard focus, the 960×600 minimum layout, Windows binary launch, and warning-free WebView navigation have been smoke-tested.
