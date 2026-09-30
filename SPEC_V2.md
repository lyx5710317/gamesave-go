# GameSave Cloud MVP V2.1 Specification

Status: execution baseline for the OpenSave fork. The supplied V2.1 product document is the product authority; this file records its repository-facing interpretation and the audited state of upstream commit `1d7476db5270e06aca0dc5a4c7e5ff669e344bc3`.

## Product position

GameSave Cloud is an open-source, Windows-first, local-first desktop tool for automatic game-save backup, version history, restore, and cross-device synchronization. Users bring their own cloud storage. The product does not operate a GameSave Cloud account system or a server that stores game saves.

Windows-first means the Windows experience, packaging, and Chinese cloud-provider path receive first priority. It does not authorize removal or degradation of OpenSave's Linux, Steam Deck, CLI, or P2P foundations.

## MVP scope

The MVP preserves and extends OpenSave rather than replacing it:

- keep Go, Wails 2, the current Svelte 5/Vite frontend, and SQLite;
- reuse game discovery, Ludusavi Manifest resolution, recursive file watching, snapshots, branches, restore, conflicts, cloud sync, P2P, and CLI;
- add a small internationalization layer, starting with English and Simplified Chinese;
- reshape the later Windows UI around Games, Cloud Backup, Activity, and Settings without deleting advanced device-sync features;
- design, verify, and then add mainland-China cloud providers as isolated integrations;
- retain upstream-compatible internal names such as `OpenSave`, `.opensave`, database names, and protocol identifiers until a separate migration is designed.

Not in the current phase: Baidu or Quark implementation, a new snapshot engine, a new account system, global rebranding, or broad UI restructuring. On 2026-09-30 the owner chose to publish a Windows-first v1.1.1 release with the remaining non-critical verification work disclosed and tracked, rather than wait for every release-preparation task. This decision does not relax fail-closed restore, credential protection, cloud inventory, conflict or overwrite safeguards. Linux and Steam Deck foundations remain in source, but their packages are not part of this Windows release.

## Architecture boundaries

The following existing packages are protected infrastructure and must be modified only for a demonstrated bug or necessary compatible extension:

```text
internal/snapshot/
internal/delta/
internal/watcher/
internal/store/
internal/p2p/
internal/presets/
```

Prefer changing an existing implementation, then extending it, and only then introducing a new core module. No duplicated snapshot, scanner, resolver, or persistence implementation is permitted.

## Cloud-provider strategy

Provider priority is:

1. Near-term mainland-China recommendation: Jianguoyun through its officially documented WebDAV endpoint and a dedicated `GameSaveGo/` directory. This is a hardened WebDAV preset, not a separate snapshot or sync engine. Its current scope is conservative backup, not verified multi-device cloud sync; see [`docs/JIANGUOYUN_PROVIDER.md`](docs/JIANGUOYUN_PROVIDER.md).
2. Later mainland-China large-capacity strategy: Baidu Netdisk, gated on public-app approval, approved API scope, and security review. Its design and provider seam remain intact.
3. Advanced: other user-supplied WebDAV destinations.
4. Deferred mainland-China candidate: Quark Netdisk. At the owner's request, do not spend the current phase on evaluation, experiments, or implementation. Revisit only if the owner reopens the work and official third-party native-client access is confirmed; it is not a release gate.
5. International: Google Drive, with existing Dropbox, OneDrive, local-folder, and webhook support retained.

Google Drive remains a conservative backup destination, not a proven atomic
multi-device snapshot store. It permits duplicate names; a complete preflight
and post-upload identity/size checks can detect some races but cannot make
name creation atomic. Show this limitation in the Cloud Backup UI and stop on
ambiguous results rather than claiming a completed upload or safe vault join.

For the current desktop release, show Jianguoyun, Google Drive, local folder, other WebDAV, and webhook as selectable setup entries. Temporarily hide Baidu, OneDrive, and Dropbox from new setup without deleting their providers, saved configurations, credentials, or roadmap. An existing hidden-provider selection must remain intact and be reported honestly rather than silently converted to another provider.

Each provider must be isolated behind a stable internal boundary, use bounded retries and rate-limit handling, and expose failures without weakening snapshot safety. No provider may require game-save bytes to transit a GameSave Cloud service.

Jianguoyun's official documentation gives a default 500 MB WebDAV single-file limit, free-account 600 requests per 30 minutes (paid: 1,500), and at most 750 entries per directory request with pagination. The public help page does not specify a pagination request/response contract or atomic conditional-write guarantees. Until these are established by official clarification and repeatable real-account tests, a full/ambiguous directory listing must fail closed, and `vault.json` conditional multi-device writes remain disabled. Snapshot create-only checks and vault metadata CAS are distinct capabilities. A successful local backup or remote ZIP inventory is not evidence of safe remote ancestry or a joined device.

## Identity model

GameSave Cloud has no username/password/email/phone identity. The target model has three independent layers:

- **Cloud identity**: the provider's OAuth identity.
- **VaultId**: a UUID created for the cloud save library and recorded in a versioned `vault.json` under the provider's GameSave Cloud application directory.
- **DeviceId**: a UUID generated per installation and registered with the vault using a human-readable device name.

Joining a second device must first scan local saves, inspect the remote vault, summarize the existing library, and require an explicit join. Local and remote states without provable ancestry become a conflict; timestamps alone never authorize a silent overwrite.

Audited upstream already has a node ID, per-device X25519 keys, and a local cryptographic vault/keyring migration. These are valuable primitives, but they are not automatically equivalent to the planned cloud `VaultId` metadata contract. Phase 3 must reconcile and reuse them before adding schema.

The reviewed reconciliation, versioned `vault.json` contract, upgrade states, and snapshot-lineage mapping are recorded in [`docs/VAULT_IDENTITY_V2.md`](docs/VAULT_IDENTITY_V2.md). It deliberately introduces no schema migration; legacy ancestry and device attribution remain unknown rather than being inferred from timestamps.

## Snapshot, restore, and conflict rules

OpenSave's snapshot model remains authoritative. The target logical history must be able to associate a snapshot with `snapshotId`, `gameId`, `deviceId`, `parentSnapshotId`, `createdAt`, and content hash, reusing equivalent existing fields where possible.

Before restoring or applying an incoming state:

1. capture a safety snapshot of the current local save;
2. verify that the safety snapshot succeeded;
3. restore the selected target atomically where practical;
4. surface failure and preserve the original current save.

If neither side is a known descendant of the other, create a conflict. Do not choose a winner solely from modification times. Branch and conflict data must stay compatible with existing OpenSave behavior and migration history.

The granular single-file API must also fail closed: stage and CRC-check the
selected regular entry before safety-snapshot retention can remove the source,
use the mapped location-relative path, and verify that an existing destination's
bytes were captured in the safety archive before replacing it. Reject a changed
local file rather than overwriting the newly observed state. Publish from a
closed, flushed sibling temporary file instead of truncating the live file.
Fixed Chinese/English error categories must not expose archive names or raw
filesystem errors. This is a scoped API bug fix, not a replacement snapshot
engine, transaction across all save locations, or proof against all concurrent
writers/filesystem aliases. Whole-directory restore retains separate limits.

Whole-snapshot `Manager.Restore` now preserves a private copy even for an empty
destination and fully reads target and newly created safety ZIP entries for
CRC/length checks before clearing saves. It rejects unmapped archived locations,
overlapping destination roots, incompatible existing target kinds and observed
mapping changes instead of reporting a partial restore as success. Existing
single-file targets accept exactly one top-level regular entry, mapped to the
configured local filename. Missing destinations are directories: legacy archive
metadata does not reliably distinguish a deleted file from a one-file folder.
The whole-restore gate also reads the current tree, including empty files,
excluded/dot files, empty directories and mapped extra roots, compares file
sizes and SHA-256 bytes with the newly created safety ZIP, then scans the
current tree again before extraction. A mismatch or observed intervening
change stops replacement with a fixed bilingual error. This is a scoped
extraction-boundary improvement with no schema/protocol change. It does not
lock out concurrent writers between the final scan and extraction, prove all
filesystem aliases, validate every legacy archive path alias, or make a
multi-root restore transactional after an I/O failure.

Branch switching with an incoming snapshot uses the same whole-restore
preflight and verified outgoing safety capture before moving the active-branch
pointer. A populated target whose archive omits a currently mapped location
must stop instead of mixing the outgoing branch's files into the target. A
deliberately empty branch first verifies its outgoing tree in a
safety snapshot, rechecks observed changes/mappings, and only then clears
the configured locations. Corrupt or unmapped incoming snapshots must not be
reported as a successful switch. An I/O failure during multi-root clearing,
or a database pointer failure after filesystem replacement, is not yet an
atomic rollback guarantee. Untracked games in a v2 backup-file import are now
skipped in both snapshot and overwrite modes: the archive's recorded path is
never used as a live restore destination. The user must first track the game
with its local save folder, then re-import so the existing tracked-game
restore safeguards apply. This temporary fail-closed policy is not an
implementation of safe untracked restore or transactional multi-root
publication.

Cloud downloads and read-only verification now reject ambiguous ZIP extraction
paths before publishing/importing an archive: duplicate paths, non-canonical
relative paths, file/directory collisions, special entries and directory bodies.
On Windows the check additionally rejects case-colliding targets, reserved
device names, trailing dots/spaces and invalid Win32 characters. Linux retains
case-sensitive names. Listed-size mismatches are rejected before decompression;
CRC checks still apply. This cloud-ingress check does not rewrite the snapshot
engine or validate every legacy local/P2P archive. It is not trusted vault
identity, ancestry, or a decompression/resource-limit guarantee. Jianguoyun
downloads no longer create a missing remote directory. See
`docs/CLOUD_ARCHIVE_SAFETY.md` for coverage and remaining manual checks.

## Automatic backup and cross-device sync

The target Windows workflow is: detect game activity, mark save changes dirty, detect game exit, wait for files to become stable, create a snapshot, then enqueue the upload. This reduces partial captures and cloud API volume. The audited watcher currently performs recursive `fsnotify`, a two-second debounce, lock/stability checks, manifest-hash deduplication, and retry before taking a snapshot directly; game-process/dirty-queue orchestration remains future work.

Synchronization must be resumable, observable, and conservative. A newly joined device cannot silently overwrite either local or remote data. Cloud and P2P transports remain separate ways to move the same safe snapshot state.

## Settings and persistence

SQLite remains the backend store, with versioned migrations required for every schema change. The initial language preference is deliberately stored under `opensave.locale` in frontend `localStorage`: it is UI-only, device-local, available before the daemon API finishes booting, and does not justify a database migration. English is the fallback; Simplified Chinese is selected from compatible system locales when no preference exists.

The desktop UI should present its settings, game-management, cloud-backup, device, and conflict-decision text in the selected language. GameSave Go is the user-facing product name; technical paths, protocol names, legacy command names, and required upstream copyright attribution remain unchanged. The separate inherited `docs/*.html` marketing site is not a localized GameSave Go release site until its content and links receive a dedicated review.

Shared confirmation dialogs resolve omitted title/action/cancel labels in the
active UI language, including cloud verification and application-password
removal. Read-only cloud verification returns fixed failure categories rather
than exposing account identifiers, credentials, request URLs, local paths or
ZIP entry names. Authentication, network, incomplete inventory, size mismatch,
archive integrity and local-I/O failures remain failures, not permission to
disable checks or overwrite data. On 2026-09-26 the owner reported that the
current candidate worked on the host but failed verification and upload on the
VM; this is an unresolved compatibility failure, not a completed release gate.

A subsequent loopback-API diagnostic found the host's global cloud switch
disabled while its Jianguoyun preset and protected-password configuration
remained present. The owner then confirmed that both host and VM browsing work
after enabling and saving the switch, then successful new-snapshot VM upload
and host verification. This resolves the reported access/upload workflow,
not restored-file/safety-snapshot recovery checks or provider CAS guarantees.
The local gate is not a provider authentication failure. The existing switch
still controls both manual cloud access and automatic mirroring. Both settings
and Cloud Backup must state that behavior explicitly, show disabled status,
and require an explicit save; never turn it on silently. Cloud inventory read
errors use fixed localized categories displayed persistently, and a failed
refresh must discard stale actionable inventory rather than show it as current
or empty. No engine/schema/credential migration is introduced by this UI fix.

Follow-up on 2026-09-27: the owner clarified that an empty local snapshot had
been restored rather than the separate cloud snapshot. After selecting cloud
restore, the owner reports successful restoration and matching content, then
successful recovery after instructions to use the automatic pre-restore safety
snapshot. Record these as owner-reported workflow PASS, not an independently
measured whole-tree/hash result or proof of an identified candidate on both
devices. A local snapshot restore never implicitly downloads a cloud object;
the UI must explain that distinction and intentional empty-state restoration.

The owner subsequently reports `game2` succeeded after the candidate's optional
single-file restore exercise. Record this as owner-reported PASS only; no exact
two-device executable hashes or independently measured file tree were supplied.

Desktop product releases use their own semantic version and stamped release-tag identity, distinct from the inherited core/peer version. The planned Windows release tag is `v1.1.1`: this is numerically newer than manually distributed `1.1` development builds, while a source-built `1.1.1` copy is distinguished from the official tagged binary by an empty release marker. Neither identity changes the peer protocol. See `docs/RELEASE_V1_1.md` for verified results and disclosed remaining risks.

Desktop update notices must come only from verified GameSave Go releases in the project's GitHub repository. A peer's inherited core version (for example `2.3.1`) is sync compatibility metadata, never a desktop update offer; peer binary installation remains disabled until a separate verified product-identity design is approved. The Windows tray follows the selected desktop UI language, while the installer asks for its own English/简体中文 language because the UI preference is not available before installation.

Official Windows downloads are served only from this repository's GitHub Releases after the explicit publication gate. The owner has chosen not to purchase Windows Authenticode signing for this personal open-source project. Each published asset must be built by GitHub Actions, covered by a release `SHA256SUMS` entry and a verifiable GitHub Artifact Attestation bound to the expected repository, workflow, ref and commit. README and release notes must disclose that unsigned Windows installers may trigger SmartScreen or be blocked by device policy, and must identify unresolved sync and dependency-scan risks. Attestation proves build provenance, not application safety or Windows publisher identity; the owner-accepted release timing does not waive runtime backup, credential, upgrade or restore safeguards.

## Security and data ownership

- User save data belongs to the user and stays local or in the user's chosen provider.
- OAuth tokens, secrets, personal data, and real account details must never be committed.
- A provider `SecretKey` must not be embedded in an open-source desktop binary.
- Prefer public-client OAuth with PKCE when officially supported; otherwise use a minimal broker only for code exchange and token refresh.
- Persist tokens using Windows-protected storage or an equivalently reviewed mechanism before shipping a China-provider integration.
- Preserve the MIT license and third-party notices.
- Log metadata conservatively; never log token values or save contents.

## Audited upstream differences

- Snapshot rows record ID, game, branch, timestamp, comment, automatic/manual state, ZIP path, and size, while content hashes live mainly in manifests/captured-file state. The requested `deviceId` and explicit `parentSnapshotId` are not snapshot-row fields.
- Restore attempts a pre-restore safety snapshot, but the current path logs and continues if that snapshot fails; V2.1 requires a hard safety gate.
- The watcher snapshots stable changes after debounce rather than waiting for game exit through a dirty/upload queue.
- Cloud handling is a central service selected by provider name, not yet a formal provider interface suited to adding several China integrations.
- OAuth access and refresh tokens are currently represented in SQLite settings. Windows-protected token storage is a release blocker for new providers.
- Existing node/device keys and local vault cryptography predate the planned cloud vault contract and must be reconciled instead of duplicated.
