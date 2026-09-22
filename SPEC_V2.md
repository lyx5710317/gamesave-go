# GameSave Cloud MVP V2.1 Specification

Status: execution baseline for the OpenSave fork. The supplied V2.1 product document is the product authority; this file records its repository-facing interpretation and the audited state of upstream commit `1d7476db5270e06aca0dc5a4c7e5ff669e344bc3`.

## Product position

GameSave Cloud is an open-source, Windows-first, local-first desktop tool for automatic game-save backup, version history, restore, and cross-device synchronization. Users bring their own cloud storage. The product does not operate a GameSave Cloud account system or a server that stores game saves.

Windows-first means the Windows experience, packaging, and Chinese cloud-provider path receive first priority. It does not authorize removal or degradation of OpenSave's Linux, Steam Deck, CLI, or P2P foundations.

## MVP scope

The MVP preserves and extends OpenSave rather than replacing it:

- keep Go, Wails 2, Svelte 4, Vite, and SQLite;
- reuse game discovery, Ludusavi Manifest resolution, recursive file watching, snapshots, branches, restore, conflicts, cloud sync, P2P, and CLI;
- add a small internationalization layer, starting with English and Simplified Chinese;
- reshape the later Windows UI around Games, Cloud Backup, Activity, and Settings without deleting advanced device-sync features;
- design, verify, and then add mainland-China cloud providers as isolated integrations;
- retain upstream-compatible internal names such as `OpenSave`, `.opensave`, database names, and protocol identifiers until a separate migration is designed.

Not in the current phase: Baidu or Quark implementation, a new snapshot engine, a new account system, global rebranding, installer/release work, or broad UI restructuring.

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

1. Mainland China primary: Baidu Netdisk.
2. Mainland China candidate: Quark Netdisk, only after official native-client access is confirmed.
3. Advanced: WebDAV.
4. International: Google Drive, with existing Dropbox, OneDrive, local-folder, and webhook support retained.

Each provider must be isolated behind a stable internal boundary, use bounded retries and rate-limit handling, and expose failures without weakening snapshot safety. No provider may require game-save bytes to transit a GameSave Cloud service.

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

## Automatic backup and cross-device sync

The target Windows workflow is: detect game activity, mark save changes dirty, detect game exit, wait for files to become stable, create a snapshot, then enqueue the upload. This reduces partial captures and cloud API volume. The audited watcher currently performs recursive `fsnotify`, a two-second debounce, lock/stability checks, manifest-hash deduplication, and retry before taking a snapshot directly; game-process/dirty-queue orchestration remains future work.

Synchronization must be resumable, observable, and conservative. A newly joined device cannot silently overwrite either local or remote data. Cloud and P2P transports remain separate ways to move the same safe snapshot state.

## Settings and persistence

SQLite remains the backend store, with versioned migrations required for every schema change. The initial language preference is deliberately stored under `opensave.locale` in frontend `localStorage`: it is UI-only, device-local, available before the daemon API finishes booting, and does not justify a database migration. English is the fallback; Simplified Chinese is selected from compatible system locales when no preference exists.

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
