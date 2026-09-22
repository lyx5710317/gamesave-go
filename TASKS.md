# GameSave Cloud MVP V2.1 Tasks

## Phase 0 — Baseline / Repository Audit

- [x] Read the supplied V2.1 execution document in full.
- [x] Import exact OpenSave upstream commit `1d7476db5270e06aca0dc5a4c7e5ff669e344bc3` and configure `upstream`.
- [x] Configure `origin` for the public `gamesave-go` fork while retaining `upstream`.
- [x] Audit README, license, dependencies, repository structure, core packages, settings, cloud providers, and tests.
- [x] Record the baseline versions, architecture, build commands, gaps, and risks.
- [x] Run the unmodified frontend tests and production build.
- [x] Run the unmodified desktop build.
- [x] Reproduce and isolate two Windows-only Go baseline failures caused by real-machine environment discovery.
- [x] Add deterministic test isolation for Windows application-data paths and Steam registry discovery.
- [x] Confirm the full Go suite and final desktop build after Phase 1 changes.
- [x] Create `SPEC_V2.md`, `AGENTS.md`, `TASKS.md`, `NOTICE.md`, and provider design documents.
- [x] Create development branch `feature/i18n`.

## Phase 1 — i18n

- [x] Add a lightweight Svelte store-based i18n layer.
- [x] Add English and Simplified Chinese catalogs.
- [x] Reserve stable locale identifiers for Traditional Chinese and Japanese.
- [x] Translate the requested first slice of App, Sidebar, and Settings.
- [x] Add Settings → General language selection with immediate switching.
- [x] Persist language per device in `localStorage` without a database migration.
- [x] Add tests for resolution, fallback, interpolation, switching, persistence, and invalid stored values.
- [x] Pass frontend tests and production frontend build.

## Phase 2 — Windows-first UI

- [x] Define the information architecture around Games, Cloud Backup, Activity, and Settings.
- [x] Move P2P/device-sync entry points to an advanced location without deleting them.
- [ ] Extend i18n page by page; do not mix this with a global internal-name migration.
  - [x] Translate the Games page shell, empty state, library controls, and game-card status.
  - [x] Translate the auto-scan dialog as one reviewed interaction.
  - [ ] Translate Cloud Backup and Activity in separate slices.
- [ ] Validate keyboard, scaling, tray, and Windows WebView behavior.
  - [x] Verify keyboard focus/navigation and the current 960×600 minimum layout in the WebView preview.
  - [x] Build and launch the Windows WebView2 desktop binary.
  - [ ] Verify native tray minimize/restore behavior on the packaged app.

## Phase 3 — Vault + Device Identity

- [ ] Reconcile current NodeID, X25519 device identity, and local keyring/vault with the V2.1 cloud model.
- [ ] Specify versioned `vault.json`, VaultId, DeviceId registration, and upgrade behavior.
- [ ] Map snapshot ancestry and device attribution onto existing data before proposing migrations.
- [ ] Require a successful safety snapshot before restore or incoming replacement.
- [ ] Design first-join scanning and conflict behavior with no silent overwrite.

## Phase 4 — Baidu Netdisk

- [ ] Verify official third-party native-app eligibility, scopes, redirects, quotas, and current API terms.
- [ ] Decide public-client OAuth versus the minimal broker.
- [ ] Introduce an isolated provider boundary without deleting existing providers.
- [ ] Implement protected token storage, refresh, retries, rate limits, upload, download, and resume.
- [ ] Add provider contract, failure, and recovery tests.
- [ ] Keep Quark at candidate status until its official access path is confirmed.

## Phase 5 — Cloud UX / Conflict / Restore

- [ ] Implement the vault discovery and explicit second-device join flow.
- [ ] Add remote-state summaries, upload queue visibility, and actionable errors.
- [ ] Implement ancestry-aware conflicts and explicit user resolution.
- [ ] Test interrupted upload/download, retry, rollback, restore, and concurrent-device scenarios.

## Phase 6 — Windows Packaging / Release

- [ ] Define versioning, signed artifacts, installer, upgrade, rollback, and release channels.
- [ ] Run clean-machine Windows installation and upgrade tests.
- [ ] Complete licenses/notices, security review, and credential-leak scanning.
- [ ] Publish only after end-to-end backup, conflict, and restore recovery tests pass.
