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
  - [x] Remove obsolete Support, Discord, and donation controls from the desktop UI; keep the current project-source link and upstream license attribution.
  - [x] Translate Settings, device/relay screens, game details, pairing/status, and conflict decisions to Simplified Chinese with paired English catalog entries and markup regression tests.
  - [x] Add a same-style "+ Add device" shortcut immediately before "+ Track folder" on the Games home screen.
  - [x] Set the GameSave Go desktop display and Windows file version to 1.1 while preserving the inherited core/peer build version until a compatible migration is designed.
  - [ ] Before publishing a v1.1 release, separate the GitHub release tag from the inherited core/peer version injected by the release workflow; verify installed builds and peer-update compatibility.
  - [ ] Replace or explicitly retire the separate upstream `docs/*.html` marketing site before publishing it as GameSave Go; it still contains old product copy and upstream download/community links. Do not treat it as a localized product site.
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

## Phase 4 — Mainland-China Cloud Providers

Near-term recommendation: Jianguoyun via official WebDAV. Baidu is the later large-capacity strategy; no Baidu code is removed or prematurely enabled. See `docs/JIANGUOYUN_PROVIDER.md` and `docs/BAIDU_PROVIDER.md`.

### Jianguoyun WebDAV preset

- [x] Add a distinct recommended Jianguoyun Cloud Backup card using the official `https://dav.jianguoyun.com/dav/` base and a fixed `GameSaveGo/` remote folder; retain other WebDAV as an advanced option and show Baidu as unavailable pending approval.
- [x] Protect Jianguoyun application passwords with Windows Credential Manager, migrate legacy plaintext official-host WebDAV rows on read, mask settings responses, preserve credentials on blank updates, and delete only on explicit disconnect. Windows migration/rollback and API masking tests use synthetic credentials.
- [x] Add a conservative 500 MB pre-upload guard, bounded metadata retries, free-tier request pacing, status-only error classification, `MKCOL` folder creation, same-name preflight, a conditional-create probe, and mandatory remote length verification. These are mocked-contract safeguards, not proof of real-account behavior.
- [x] Add a fail-closed standard WebDAV `MOVE Overwrite: F` backup-only fallback when the conditional-PUT probe is unsupported: test harmless collision probes, stage under an unpredictable name, verify staged and final objects, preserve a concurrent destination, and expose only controlled method/status diagnostics. A later readback build completed an owner-reported real-account upload/restore happy path; exact conditional semantics remain unproven.
- [x] Fail closed when a Jianguoyun directory response reaches 750 entries, contains an unrecognized page marker, duplicates, malformed XML, or an unsafe href. Do not treat such a listing as an empty/complete remote inventory.
- [ ] Obtain the official Jianguoyun WebDAV pagination request/response contract and implement complete bounded enumeration (including interrupted, repeated, and cycling pages). Until then, a full or ambiguous directory remains blocked; no claim of full-pagination support.
- [ ] Run a synthetic and then real-account compatibility matrix for free/paid request limits, 500 MB boundary, folder creation races, 401/403/404/412/429/507, network interruption, ignored conditions, concurrent same-name PUT, HEAD size and full-GET readback after ambiguous HEAD, ETag, Range and locks. The owner reports successful small-snapshot upload and cross-device restore, but that does not complete this matrix. Record PASS/FAIL/BLOCKED without committing credentials.
- [ ] Prove snapshot create-only semantics on a real account across concurrent devices. A local conditional-write probe is not sufficient proof for production multi-device publishing.
- [ ] Prove strong provider CAS for `vault.json` separately before any Jianguoyun remote-vault write or second-device join. Keep the UI explicitly backup-only until ancestry, device registration, and conflict resolution are complete.
- [ ] Review per-process request pacing versus multiple clients, and add a durable recovery queue with remote identity verification before public release.

### Baidu Netdisk

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
- [x] Defer Quark evaluation, experiments, and implementation at the owner's request. Keep its candidate design record, but do not treat Quark as a current-phase task or release blocker; no Quark API implementation or undocumented path has been added.

## Phase 5 — Cloud UX / Conflict / Restore

Current priority (2026-09-25): Jianguoyun official-WebDAV backup safety after the owner's successful small-snapshot cross-device test, then remaining cloud-vault and conflict safeguards. Baidu remains the later large-capacity path. Dropbox, OneDrive, and Quark development are deferred at the owner's request.

- [x] Temporarily hide Baidu, OneDrive, and Dropbox setup cards without removing providers or rewriting existing selections; explain an existing hidden selection and prevent accidental re-save from its hidden panel.
- [ ] Revisit these three setup entries only when the owner requests them and provider-specific release gates are met.

- [ ] Implement the vault discovery and explicit second-device join flow.
  - [x] Expose a read-only local save scan in Cloud Backup, using the existing first-join scanner; remote comparison and confirmation remain pending.
  - [x] Show a read-only inventory of recognizable snapshots in the currently saved cloud destination beside the local scan. This is not `vault.json` discovery, verified ancestry, or permission to join or overwrite.
  - [x] Inspect `vault.json` read-only for Jianguoyun and local-folder destinations, with bounded validation and distinct missing/invalid/unsupported/unavailable states. Do not create the remote folder, expose device identities, register a device, or treat a readable document as proof of CAS or ancestry. Other providers remain unsupported by this inspection.
  - [x] Show a display-only warning for game IDs present in both a completed local scan and a recognizable remote ZIP inventory. Reject duplicate recognizable snapshot names and malformed inventory instead of presenting an ordinary browse/preview list. Matching or differing IDs do not establish content equality, ancestry, or permission to join.
- [ ] Add remote-state summaries, upload queue visibility, and actionable errors.
  - [x] Show bounded, process-lifetime status for existing automatic and manual uploads; report partial manual failures separately from already-current snapshots, including CLI exit status. This is not a durable retry queue.
  - [x] Surface a controlled Jianguoyun safety-check step and HTTP status in transfer activity when a connected account cannot prove no-overwrite writes; never expose raw responses or credentials. The owner's subsequent readback-build upload/restore happy path passed, but exact server semantics remain unproven.
  - [x] Add an optional read-only per-snapshot cloud verification action: stage a remote ZIP in OS temp storage, validate ZIP/CRC and listed size, compare bytes with a same-name local archive when present, and report the three outcomes without publishing or restoring. This is not trusted vault identity or ancestry and consumes provider download traffic.
  - [x] Stop manual sync from treating a same-name, same-size object as verified or overwriting a same-name object of different size; surface these unverified objects for explicit review. This is a listing-based guard, not an atomic create-if-absent guarantee. Provider-atomic no-overwrite uploads and verified remote identity remain pending.
  - [x] Apply the same fail-closed name check to automatic uploads and recheck immediately before manual writes; classify collisions in transfer activity. Local-folder writes now use exclusive creation, and WebDAV sends `If-None-Match: *`. Remote listing can still be incomplete or race across devices; provider-specific atomic guarantees and verified identity remain pending.
  - [x] Enumerate every Google Drive snapshot-list page and reject incomplete or cycling pagination before using the listing for upload decisions.
  - [x] Reuse the complete Google Drive listing for restores; reject duplicate snapshot names, missing IDs, and incomplete inventories before downloading any bytes. This does not establish verified remote identity or atomic name creation.
  - [x] Enumerate Dropbox and OneDrive snapshot-list pages before upload decisions; reject missing/repeated Dropbox cursors, unexpected Dropbox listing conflicts, unsafe/repeated OneDrive next-page URLs, and page failures. Provider-atomic create-only writes and verified remote identity remain pending.
  - [x] Use provider-enforced create-only uploads for Dropbox (strict add, no autorename) and OneDrive (fail-on-conflict for simple and session uploads), including late-conflict tests. Google Drive name uniqueness and verified remote identity remain unresolved; webhook destinations cannot provide a general create-only guarantee.
  - [x] Suspend automatic remote retention pruning until the cloud vault can verify ownership and ancestry; local retention and explicit cloud delete remain available. Unknown remote history is preserved even when the local automatic-snapshot limit is low.
  - [x] Cover simultaneous Dropbox and OneDrive uploads after both clients observe an empty remote list; provider-enforced conflict leaves exactly one remote writer. This does not establish Google Drive atomicity or ancestry-aware resolution.
  - [ ] Add a durable, resumable queue with verified remote state, retry and recovery after the provider contract is approved.
- [ ] Implement ancestry-aware conflicts and explicit user resolution.
- [ ] Test interrupted upload/download, retry, rollback, restore, and concurrent-device scenarios.
  - [x] Pair a Windows 11 VM over LAN and verify a dummy save reaches an isolated host profile with matching file hashes; preserve both sides and require explicit choice when a conflict is reported.
  - [x] Reject automatic peer-game tracking from an unmapped temporary directory, as the Windows VM test showed that profile substitution could silently select a different empty folder. An explicit path translation or manual game path remains available.
  - [x] Treat a peer's required manual save path as an actionable non-retryable state, without advancing its last-synced time; show the affected device on manual sync instead of claiming success.
  - [ ] Reproduce the VM's rapid successive-file conflict in an automated two-device test before changing conflict detection. A regression test for two rapid one-sided additions, including an empty file, passed 20 repetitions but did not reproduce the VM conflict; do not relax the ancestry safety rule based on matching timestamps or a shared file alone.

## Phase 6 — Windows Packaging / Release

- [ ] Define versioning, signed artifacts, installer, upgrade, rollback, and release channels.
- [ ] Run clean-machine Windows installation and upgrade tests.
- [ ] Complete licenses/notices, security review, and credential-leak scanning.
- [ ] Publish only after end-to-end backup, conflict, and restore recovery tests pass.
