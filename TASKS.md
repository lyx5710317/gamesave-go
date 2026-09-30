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
    - [x] Stop stamping the desktop tag onto the inherited core/peer version in Windows and Linux build jobs; retain build timestamps and add a tag/metadata preflight. This is static and local-build coverage, not a GitHub-runner or installed-build result.
    - [ ] Verify the tagged workflow and installed candidate against an older peer on two Windows devices.
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
  - [x] Localize shared confirmation defaults (including verification and application-password removal); add fixed, localized verification failure categories with no raw credential, account, URL, path or ZIP-entry data. Cover error-code mapping, read-only failure cleanup and unchanged local archives.
  - [x] Diagnose the host's cloud-read failure as a disabled local cloud gate, not lost credentials; expose the same explicit global enable switch in Cloud Backup and clarify its automatic-upload side effect in Settings. Distinguish disabled status from configured/connected, return fixed inventory-read error codes, keep localized failure instructions visible and discard stale actionable inventory after failed refresh. Preserve hidden providers and do not silently enable cloud operations.
  - [x] Stop manual sync from treating a same-name, same-size object as verified or overwriting a same-name object of different size; surface these unverified objects for explicit review. This is a listing-based guard, not an atomic create-if-absent guarantee. Provider-atomic no-overwrite uploads and verified remote identity remain pending.
  - [x] Apply the same fail-closed name check to automatic uploads and recheck immediately before manual writes; classify collisions in transfer activity. Local-folder writes now use exclusive creation, and WebDAV sends `If-None-Match: *`. Remote listing can still be incomplete or race across devices; provider-specific atomic guarantees and verified identity remain pending.
  - [x] Enumerate every Google Drive snapshot-list page and reject incomplete or cycling pagination before using the listing for upload decisions.
  - [x] Reuse the complete Google Drive listing for restores; reject duplicate snapshot names, missing IDs, and incomplete inventories before downloading any bytes. This does not establish verified remote identity or atomic name creation.
  - [x] Reject ambiguous/missing auto-managed Google Drive folders instead of choosing the first name match; recheck after folder creation. Resume uncertain Drive chunks only after a session-status query, and verify completed upload metadata plus the unique ID/size in a fresh listing. These fail-closed checks do not make Drive name creation atomic or prove provider consistency on real accounts.
  - [ ] Verify Google Drive's folder, resumable-upload, and post-upload-list behavior on a disposable real account, including two simultaneous writers and an ambiguous completion. Follow `docs/GOOGLE_DRIVE_BACKUP_SAFETY.md`; keep the UI backup-only and do not promise safe multi-device sync meanwhile.
  - [x] Enumerate Dropbox and OneDrive snapshot-list pages before upload decisions; reject missing/repeated Dropbox cursors, unexpected Dropbox listing conflicts, unsafe/repeated OneDrive next-page URLs, and page failures. Provider-atomic create-only writes and verified remote identity remain pending.
  - [x] Use provider-enforced create-only uploads for Dropbox (strict add, no autorename) and OneDrive (fail-on-conflict for simple and session uploads), including late-conflict tests. Google Drive name uniqueness and verified remote identity remain unresolved; webhook destinations cannot provide a general create-only guarantee.
  - [x] Suspend automatic remote retention pruning until the cloud vault can verify ownership and ancestry; local retention and explicit cloud delete remain available. Unknown remote history is preserved even when the local automatic-snapshot limit is low.
  - [x] Cover simultaneous Dropbox and OneDrive uploads after both clients observe an empty remote list; provider-enforced conflict leaves exactly one remote writer. This does not establish Google Drive atomicity or ancestry-aware resolution.
  - [ ] Add a durable, resumable queue with verified remote state, retry and recovery after the provider contract is approved.
- [ ] Implement ancestry-aware conflicts and explicit user resolution.
- [ ] Test interrupted upload/download, retry, rollback, restore, and concurrent-device scenarios.
  - [x] Harden the existing single-file restore API: fail closed on safety-snapshot creation/verification failure; stage before retention can delete the selected source; verify selected-entry CRC and size; reject duplicate/special entries; restore extra-location files without their archive prefix; compare protected old bytes and stop on an observed concurrent change; publish without truncating the live destination. Add bilingual fixed diagnostics and explain local versus cloud restore. Synthetic tests reproduce old failures; this does not prove globally atomic restore or all filesystem aliases.
  - [x] Record owner-reported VM upload → host verification → explicit cloud restore with matching content, followed by successful recovery after the automatic-safety-snapshot instructions (2026-09-27). The earlier empty-directory report concerned a local empty snapshot, not a cloud-download delay. Exact two-device binary identity, full-tree SHA-256 evidence and real-provider conditional-write races remain open.
  - [x] Localize known generated restore-safety snapshot comments for easier recovery, without changing stored snapshot metadata or user-written notes; test both languages and preservation of custom comments.
  - [x] Record the owner's subsequent successful `game2` granular-restore test (2026-09-27), without treating it as independent candidate-hash/full-tree evidence.
  - [x] Record the owner's successful `game2` whole-directory/safety-recovery exercise after the next candidate handoff and authorization to upload the source branch; exact VM/host artifact identity and independently measured trees remain unverified.
  - [x] Add whole-restore preflight at the existing extraction boundary: privately copy the target, read all target/safety entries for CRC and size before clearing, reject unmapped or overlapping archived roots and incompatible target types, and recheck observed mappings. Correct missing-folder one-entry extraction and configured single-file basename mapping; add fixed bilingual API/UI errors. The new regressions reproduce earlier data loss/misplacement and false partial-success behavior. No schema or restore-engine rewrite.
  - [x] Compare the observed whole current tree with the pre-restore safety ZIP: file size/SHA-256, empty files, excluded/dot files, empty directories and mapped extra roots. Re-scan immediately before extraction, reject observed changes, and show fixed bilingual diagnostics; synthetic round-trip and deliberately incomplete-but-readable safety ZIP tests cover the gate. This is not a filesystem writer lock or transaction.
  - [x] Record the owner's 2026-09-28 report that restoration succeeded and snapshots synchronized on both devices after the whole-current-tree safety-capture build. This is a manual happy-path PASS, not independent proof of matching executable hashes, full-tree hashes, cloud-vault ancestry or concurrent-write guarantees.
  - [ ] Complete legacy/local archive path-alias validation and test staged multi-location publication/recovery after mid-write I/O failure and concurrent writers. The final scan-to-extraction interval remains vulnerable to a concurrent writer; missing deleted-file destinations need an explicit compatible destination-kind design before file mode can be inferred safely.
  - [x] Route populated-branch checkout through verified `Manager.Restore`, rejecting an incoming archive that omits a currently mapped location; verify the outgoing current tree/safety snapshot before clearing an intentionally empty branch. Keep the active branch unchanged on preflight failure; cover corrupt/incomplete incoming ZIP, incomplete readable safety ZIP and observed concurrent additions. This does not make multi-root publication or the final branch-pointer update transactional.
  - [x] Disable the separate untracked v2 backup-file overwrite path in both import modes; skip before staging or writing any archive entry and show bilingual guidance to track the game and re-import. Regression coverage checks existing and absent destinations, while tracked-game overwrite remains available.
  - [ ] If untracked backup-file restore is ever re-enabled, design and test explicit destination selection, verified safety capture, staged publication and recovery. The temporary skip does not complete transactional multi-root restore.
  - [x] Reject ambiguous cloud ZIP paths before archive publication/import or save replacement; cover duplicate names, non-canonical relative paths, file/directory conflicts, special entries and Windows case/device-name aliases. Check listed size before decompression, keep CRC failures free of entry names, and preserve ordinary/multi-location archives and Linux case sensitivity. API regression tests confirm no save changes, remote changes, imported rows or leftover staging files on rejection.
  - [x] Keep Jianguoyun downloads/verification read-only when the destination folder has disappeared; report missing content instead of recreating the remote folder. Add a localized, fixed unsafe-archive restore diagnostic without returning untrusted entry names.
  - [x] Repeat simultaneous publication of two verified downloads to one local archive path: different bytes keep one winner and report one conflict; identical bytes can both succeed without partial output. This tests Windows local publication only, not provider atomicity or FAT/network filesystem behavior.
  - [ ] Repeat the identified candidate's synthetic Windows VM round trip with recorded full-tree/size/SHA-256 comparison. Browsing, new VM upload, host verification, explicit cloud restore with matching contents and subsequent safety-recovery success are now owner-reported PASS; exact executable hashes on both devices and independently recorded full-tree/recovery-marker evidence remain open. Do not bypass safety checks. Follow `docs/CLOUD_ARCHIVE_SAFETY.md`; real-provider condition races and ETag/CAS remain separately BLOCKED.
  - [x] Pair a Windows 11 VM over LAN and verify a dummy save reaches an isolated host profile with matching file hashes; preserve both sides and require explicit choice when a conflict is reported.
  - [x] Reject automatic peer-game tracking from an unmapped temporary directory, as the Windows VM test showed that profile substitution could silently select a different empty folder. An explicit path translation or manual game path remains available.
  - [x] Treat a peer's required manual save path as an actionable non-retryable state, without advancing its last-synced time; show the affected device on manual sync instead of claiming success.
  - [ ] Reproduce the VM's rapid successive-file conflict in an automated two-device test before changing conflict detection. A regression test for two rapid one-sided additions, including an empty file, passed 20 repetitions but did not reproduce the VM conflict; do not relax the ancestry safety rule based on matching timestamps or a shared file alone.
  - [ ] Verify deletion propagation under Linux `-race` and repeated two-device soak. PR #2 reported that `slot4.sav` returned after B deleted it. Inspection found that LAN/WAN delete handlers ignored `os.Remove` errors and still reported success, while the sync engine ignored transport deletion errors and could advance lineage. Those paths now fail visibly instead of claiming convergence; WAN deletion also resolves the named save root. Focused local tests pass, but the intermittent soak needs independent race/VM confirmation.

## Phase 6 — Windows Packaging / Release

- [ ] Define versioning, attested/checksummed artifacts, installer, upgrade, rollback, and release channels.
  - [x] Record the separate desktop/core version identities, stable-only candidate policy, publish gate, and Windows validation checklist in `docs/RELEASE_V1_1.md`.
  - [x] Keep the GitHub Release job behind an explicit repository variable and use GameSave Go Windows installer/metadata branding without renaming compatibility paths. The owner chose a disclosed unsigned-release policy instead of paid Windows signing; the gate remains off while other checks are open.
  - [x] Verify the new Actions attestation workflow on branch candidates: [run 36543874720](https://github.com/lyx5710317/gamesave-go/actions/runs/36543874720) attested final Windows files after metadata stamping; [run 36546639311](https://github.com/lyx5710317/gamesave-go/actions/runs/36546639311) additionally logged SHA-256 for all four. A separate job downloaded and verified their bytes against the repository/workflow/ref/commit. Linux, Flatpak and manifest attestations still require a tagged-release validation.
  - [x] Add a branch-ref, version-checked manual Windows candidate build that cannot publish a release, sign as an official build, or stamp a branch name as the desktop release tag; this is static/local-test coverage only.
  - [x] Execute the branch-ref unsigned Windows candidate workflow on GitHub runners; runs `36534491985` and `36537434811` passed preflight/Windows and skipped public-release jobs. The final artifact is present in Actions, but its downloaded binaries/hashes were not independently inspected; these runs predate attestation.
  - [ ] Verify an inspected unsigned candidate as an installed binary in a clean Windows VM before considering a version tag. The attested release workflow was merged to `main` in PR #1; `main` CI run 36564043934 passed. Candidate [36589803878](https://github.com/lyx5710317/gamesave-go/actions/runs/36589803878) had matching hashes and verified attestation, but the owner's 2026-09-30 VM launch showed a black window and English tray; it is superseded. Replacement candidate [36663430295](https://github.com/lyx5710317/gamesave-go/actions/runs/36663430295) passed Windows build, attestation and independent verification; the owner reports homepage and Chinese tray PASS in a VM after removing the old program. This is not an upgrade/rollback or save/credential-retention check, and the guest hash was not reported. A separate Windows race CI suite failed on game-ID collision and watcher timeout; diagnose before publication.
  - [ ] Execute the tagged workflow on GitHub runners only after remaining gates pass; inspect every published asset, SHA-256 entry and attestation. Keep GitHub Releases as the sole official download channel.
  - [x] Bind the desktop one-click update to the selected asset and `SHA256SUMS` from the same canonical GameSave Go GitHub release; bound the checksum response, verify the declared size and SHA-256 before extraction/swap/installer launch, and fail closed on missing, duplicate, wrong-origin, oversized, or changed assets. This is synthetic-test coverage, not an attested-release or installed-upgrade result.
  - [ ] Verify candidate installer behavior on a clean VM and the actual published SHA-256/attestation verification instructions. A checksum alone is not independent provenance; Artifact Attestations do not satisfy Windows Authenticode or SmartScreen. Test real GitHub asset redirects and the portable/installer fallback before enabling public updates.
  - [x] Record the owner's 2026-09-29 decision not to purchase Windows code-signing for this personal open-source project. Document unsigned-publisher/SmartScreen limitations in README and release notes; do not imply provenance attestation removes them.
  - [x] Choose `v1.1.1` rather than `v1.1.0` so already distributed 1.1 development binaries see the patch as newer; add a separate linker-stamped desktop release tag so same-version source builds can see the official release without an official build offering itself. Unit and linker-stamp tests cover the decision logic; installed-build upgrade checks remain open.
  - [x] Remove inherited peer `2.3.1` binary-update offers from the desktop Devices view/API and reject the legacy peer-install binding; keep P2P save sync and check only this project's GitHub releases for desktop updates. Synthetic regression tests cover separation, not a published release.
  - [ ] Validate both installer languages and upgraded credentials in a VM. The owner reported a fresh-Windows-VM PASS for Chinese installation, Chinese tray and synthetic-save retention after uninstall on 2026-09-29; English pages, upgraded credential usability and independent hashes remain unverified.
  - [ ] Test installer upgrade and binary rollback without reverting user data or credentials.
    - [x] Record the owner's offline Windows VM 1.1.0 → 1.1.1 → 1.1.0 → 1.1.1 manual round trip: the synthetic game, pre/post-upgrade snapshots and B-version file remained visible, and Jianguoyun's protected-password indicator remained configured. VMware snapshots were emergency backups, not used for the rollback. This is UI/content observation, not independently measured guest hashes or live credential authentication.
- [ ] Run clean-machine Windows installation and upgrade tests.
  - [ ] Reproduce or explain the first 1.1.0 VM launch showing a local-service error after a downgrade from 1.1.1, followed by a successful home-screen launch after VM reboot. The selected VM snapshot already contains three tracked games and earlier cloud/P2P activity, so use a genuinely fresh Windows profile or VM state for separate clean-install evidence. Record the actual startup reason rather than attributing old network errors to it.
  - [x] Owner-reported pristine-Windows-VM candidate check on 2026-09-29: first installation, Chinese installer/tray and synthetic-save retention after uninstall all passed. No independent guest evidence or hashes were supplied; this does not cover upgrade/rollback, credential usability, or signing.
- [ ] Complete licenses/notices, security review, and credential-leak scanning.
  - [x] Run a local pre-release source/artifact spot check on 2026-09-29: no tracked save/credential/binary extension or common private-key/token markers was found; the local portable Wails candidate was unsigned and had empty Windows product/file version fields. A local unsigned NSIS candidate later built with correct installer version fields, but no public signing or VM behavior was established. This limited scan is not a full secret or malware audit.
  - [x] Upgrade the frontend Svelte/Vite/Vitest build chain with a fresh lockfile: local `npm ci`, 117 tests, production build, Wails build, and full `npm audit` passed; the audit reports zero findings on 2026-09-29. The newer toolchain requires Node 22.12+ for tests (CI uses Node 22); the older local Node 20.10 is insufficient.
  - [ ] Complete a Go dependency vulnerability scan when the official Go vulnerability service is reachable; a fresh `govulncheck` download again timed out at `proxy.golang.org` on 2026-09-29. Do not mark this scan as passed.
  - [x] On Windows, protect generic WebDAV passwords, custom OAuth client secrets and custom request headers in Credential Manager with legacy SQLite migration and rollback tests; settings responses now expose configured flags only. The non-Windows implementation retains its prior storage behavior and needs a separate secure-storage design before claiming equivalent confidentiality on Linux/Steam Deck. Old URL-bound generic WebDAV entries are not reused for a different URL but still need lifecycle cleanup review.
  - [ ] Independently test store URI/path escaping for custom data directories containing `#` or `?`; the unnamed empty-payload temporary fixture in granular restore tests produced a database-ID collision, avoided by a descriptive fixture name. This is a review lead, not a verified general fix; no store or schema change was made.
  - [x] Confirm PR #2 race-CI after background conflict shutdown is drained: the first PR Linux run detected `snapshot.Manager.inFlight.Add` racing shutdown `Wait` from an API conflict-resolution goroutine. The server now cancels, rejects new work and waits before daemon/store shutdown; latest commit `31f447f` passed both Linux and Windows `-race` CI (push and PR runs). This does not replace installed mixed-device testing.
- [ ] Publish only after end-to-end backup, conflict, and restore recovery tests pass.
