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
- [x] Extend i18n page by page; do not mix this with a global internal-name migration.
  - [x] Translate the Games page shell, empty state, library controls, and game-card status.
  - [x] Translate the auto-scan dialog as one reviewed interaction.
  - [x] Translate Cloud Backup and Activity in separate slices.
- [x] Apply the GameSave Go user-facing brand and supplied icon while preserving compatibility identifiers.
- [x] Point the in-app repository and release checks at the `gamesave-go` fork, with legacy update-asset fallback.
- [x] Validate keyboard, scaling, tray, and Windows WebView behavior.
  - [x] Verify keyboard focus/navigation and the current 960×600 minimum layout in the WebView preview.
  - [x] Build and launch the Windows WebView2 desktop binary.
  - [x] Verify native tray hide/restore behavior on the packaged app.

## Phase 3 — Vault + Device Identity

- [x] Reconcile current NodeID, X25519 device identity, and local keyring/vault with the V2.1 cloud model. See `docs/VAULT_IDENTITY_V2.md`.
- [x] Specify versioned `vault.json`, VaultId, DeviceId registration, and upgrade behavior.
- [x] Map snapshot ancestry and device attribution onto existing data before proposing migrations.
- [x] Require a successful safety snapshot before restore or incoming replacement.
- [x] Design and implement provider-independent first-join scanning and conflict previews with no silent overwrite. See `internal/vaultmeta` and `docs/VAULT_IDENTITY_V2.md`.

## Phase 4 — Baidu Netdisk

- [ ] Verify official third-party native-app eligibility, scopes, redirects, quotas, and current API terms. Documentation findings are recorded in `docs/BAIDU_PROVIDER.md`; the owner reports the app is online for personal use only. Public-app approval, granted APIs, and approved redirect remain open.
- [x] Decide public-client OAuth versus the minimal broker. Documented code/device flows require `SecretKey`, so use a minimal OAuth broker, pending security review and provider approval.
- [x] Introduce an isolated provider boundary without deleting existing providers. Snapshot operations now route through `internal/cloud.Provider`; existing providers remain behind a compatibility adapter.
- [ ] Implement protected token storage, refresh, retries, rate limits, upload, download, and resume.
  - [x] Add a Windows OS-protected token backend with rotation, isolation, corruption, deletion, and no-plaintext-fallback tests (`internal/cloud/protectedtokens`). It is not yet connected to Baidu OAuth; existing providers are unchanged.
  - [ ] Integrate the protected backend with approved Baidu OAuth and implement refresh, retries, rate limits, and transfers after app approval and security review.
- [ ] Add provider contract, failure, and recovery tests.
  - [x] Stage and verify cloud-restored ZIPs before publication, reject local/remote same-name conflicts, and test interrupted/corrupt/path-unsafe downloads without touching live saves or existing local backups.
  - [x] Define an optional `vault.json` conditional-write contract and test stale concurrent writes, provider switches, swapped version tokens, invalid revisions, device identity changes, and revocation rollback; no existing provider is changed.
  - [ ] Add Baidu adapter contract tests for OAuth, multipart resume, rate limits, quota, and recovery once approved API behavior is confirmed.
- [x] Keep Quark at candidate status until its official access path is confirmed. No Quark API implementation or undocumented path has been added.

## Phase 5 — Cloud UX / Conflict / Restore

- [ ] Implement the vault discovery and explicit second-device join flow.
  - [x] Expose a read-only local save scan in Cloud Backup, using the existing first-join scanner; remote comparison and confirmation remain pending.
  - [x] Show a read-only inventory of recognizable snapshots in the currently saved cloud destination beside the local scan. This is not `vault.json` discovery, verified ancestry, or permission to join or overwrite.
- [ ] Add remote-state summaries, upload queue visibility, and actionable errors.
  - [x] Show bounded, process-lifetime status for existing automatic and manual uploads; report partial manual failures separately from already-current snapshots, including CLI exit status. This is not a durable retry queue.
  - [x] Stop manual sync from treating a same-name, same-size object as verified or overwriting a same-name object of different size; surface these unverified objects for explicit review. This is a listing-based guard, not an atomic create-if-absent guarantee. Provider-atomic no-overwrite uploads and verified remote identity remain pending.
  - [x] Apply the same fail-closed name check to automatic uploads and recheck immediately before manual writes; classify collisions in transfer activity. Local-folder writes now use exclusive creation, and WebDAV sends `If-None-Match: *`. Remote listing can still be incomplete or race across devices; provider-specific atomic guarantees and verified identity remain pending.
  - [x] Enumerate every Google Drive snapshot-list page and reject incomplete or cycling pagination before using the listing for upload decisions.
  - [x] Enumerate Dropbox and OneDrive snapshot-list pages before upload decisions; reject missing/repeated Dropbox cursors, unexpected Dropbox listing conflicts, unsafe/repeated OneDrive next-page URLs, and page failures. Provider-atomic create-only writes and verified remote identity remain pending.
  - [x] Use provider-enforced create-only uploads for Dropbox (strict add, no autorename) and OneDrive (fail-on-conflict for simple and session uploads), including late-conflict tests. Google Drive name uniqueness and verified remote identity remain unresolved; webhook destinations cannot provide a general create-only guarantee.
  - [x] Suspend automatic remote retention pruning until the cloud vault can verify ownership and ancestry; local retention and explicit cloud delete remain available. Unknown remote history is preserved even when the local automatic-snapshot limit is low.
  - [ ] Add a durable, resumable queue with verified remote state, retry and recovery after the provider contract is approved.
- [ ] Implement ancestry-aware conflicts and explicit user resolution.
- [ ] Test interrupted upload/download, retry, rollback, restore, and concurrent-device scenarios.

## Phase 6 — Windows Packaging / Release

- [ ] Define versioning, signed artifacts, installer, upgrade, rollback, and release channels.
- [ ] Run clean-machine Windows installation and upgrade tests.
- [ ] Complete licenses/notices, security review, and credential-leak scanning.
- [ ] Publish only after end-to-end backup, conflict, and restore recovery tests pass.
