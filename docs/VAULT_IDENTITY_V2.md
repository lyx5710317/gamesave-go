# Vault and Device Identity Contract

Status: Phase 3 design baseline for GameSave Cloud MVP V2.1. This document
defines the compatibility boundary before any cloud-vault schema migration or
provider implementation is added.

## Decisions

GameSave Go has three independent identities:

1. **Cloud identity** belongs to the storage provider. It authorizes access to
   the user's own provider space and is never used as a GameSave Go account.
2. **VaultId** identifies one cloud save library. It is not derived from an
   OAuth account, a device key, or a folder path.
3. **DeviceId** identifies one installation registered with a vault. The
   human-readable device name is only a label and is never an identifier.

The existing OpenSave foundations remain authoritative where they already
provide the required property. We do not create parallel identities or keys.

| Existing primitive | Property already provided | V2.1 decision |
| --- | --- | --- |
| `settings.node_id` | Stable random per-installation identity used by P2P discovery, pairing, and peer rows | Reuse its 128-bit random payload as the cloud DeviceId. Keep the `node_...` value unchanged for P2P compatibility. |
| `settings.device_private_key` / `device_public_key` | Long-lived X25519 device identity, generated lazily | Reuse for device registration and vault-key wrapping. The public half may be published; the private half never leaves the device. |
| `settings.vault_id` | Stable 128-bit identifier bound into encrypted payload context | Reuse exactly. New cloud vaults use UUID v4 text; existing 32-hex values remain valid legacy VaultIds and must not be reformatted in encrypted history. |
| `vault_keys` and `vault_current_key_id` | Append-only local keyring with rotation epochs | Reuse. `vault.json` never contains raw vault keys. Wrapped keys belong in per-device key objects outside the metadata file. |
| provider OAuth identity | Access to a user-owned provider directory | Keep separate. It neither names nor owns the logical vault. |

### DeviceId mapping

Current NodeIDs are generated as `node_` plus the 32 hexadecimal digits of a
UUID. Cloud metadata writes those same 128 bits in canonical UUID form while
also retaining `nodeId` as the compatibility bridge. This is a lossless
formatting change, not a second identity.

A legacy imported NodeID that does not contain a 128-bit hexadecimal payload
cannot be converted this way. Such an installation will require one persisted
DeviceId in a future forward migration. It must never generate a replacement
on each start. No such migration is introduced in this phase.

The X25519 public key authenticates a device registration but is not the
DeviceId. If the same DeviceId appears with another public key, joining stops
for an explicit identity/reinstallation decision; metadata must not silently
replace the pinned key.

### VaultId compatibility

New vaults use canonical UUID v4 strings. Existing `settings.vault_id` values
were generated from 16 random bytes and encoded as 32 lowercase hexadecimal
characters. They remain valid legacy VaultIds and are written unchanged.

The exact string matters because it is associated data for encrypted payloads.
Reformatting an existing value, even to an equivalent hyphenated UUID, would
make old ciphertext fail authentication. Each stored blob therefore continues
to carry the exact VaultId context that sealed it.

## `vault.json` version 1

The provider's GameSave Go application directory contains one metadata file:

```json
{
  "schemaVersion": 1,
  "vaultId": "80cc57d6-03c4-4db4-bf2d-8c93a5ca4efa",
  "revision": 7,
  "createdAt": "2026-09-22T10:00:00Z",
  "updatedAt": "2026-09-22T10:15:00Z",
  "devices": [
    {
      "deviceId": "7e2ae42d-4672-4e48-b840-21f72afbe3dc",
      "nodeId": "node_7e2ae42d46724e48b84021f72afbe3dc",
      "name": "Gaming PC",
      "identityPublicKey": "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=",
      "registeredAt": "2026-09-22T10:00:00Z",
      "revokedAt": null
    }
  ]
}
```

Rules:

- `schemaVersion` is required. Unknown newer versions are not rewritten; the
  client stops with an upgrade-required error.
- `vaultId` is immutable after creation. Version 1 accepts canonical UUID v4
  text and the exact 32-hex legacy form described above.
- `revision` is a monotonically increasing metadata revision. It detects
  stale writes; it is not a snapshot sequence and cannot resolve save
  conflicts.
- `createdAt`, `updatedAt`, `registeredAt`, and `revokedAt` are RFC 3339 UTC
  metadata. Timestamps never choose a winning save.
- `devices` is keyed by `deviceId`. Device names are untrusted display text,
  limited before writing, and never used in provider paths.
- `identityPublicKey` is the existing X25519 public key encoded in base64. Raw
  vault keys, private keys, OAuth tokens, provider account identifiers, local
  paths, and save contents are forbidden in this file.
- Device revocation is represented by `revokedAt`; registrations are not
  silently deleted because stale devices must remain distinguishable from
  never-seen devices.
- Wrapped vault keys are stored separately per device and are replaced only
  through a reviewed key-rotation flow.

Metadata updates use provider conditional writes (ETag/revision or the closest
official equivalent): read, validate, merge by DeviceId, increment `revision`,
then conditionally replace. A concurrent-write failure causes a reread and
merge; it never falls back to last-writer-wins by timestamp.

`internal/cloud.VaultMetadataProvider` now defines the optional read/compare-
and-swap capability for the exact provider account and `vault.json` object.
The cloud service validates the remote document and an opaque provider version
token, enforces one-step revision changes, immutable vault identity, retained
device registrations, and monotonic revocation. A caller also cannot pair old
metadata with a newer provider version token to bypass the required reread and
merge. Stale writes return a conflict without an automatic retry. Existing
providers do not implement this optional capability, and no production provider
metadata write is enabled yet. A Baidu adapter must prove strong conditional-
write semantics before implementing it;
the wrapped-key prerequisite for registering a device remains separate.

## Discovery, creation, and upgrade behavior

| Local state | Remote state | Required behavior |
| --- | --- | --- |
| No vault | No `vault.json` | Offer to create a vault after the provider is connected. Create metadata and the initial wrapped key atomically enough that an incomplete vault is detectable. |
| Existing local vault | No `vault.json` | On explicit cloud enable, offer to publish this vault. Preserve its VaultId and keyring; do not mint a replacement. |
| No local vault | Valid remote vault | Scan local saves, summarize both libraries, then require an explicit join. Do not upload, restore, register, or overwrite before acceptance. |
| Same local and remote VaultId | Valid remote vault | Validate the current device registration and merge it with a conditional metadata write. |
| Different local and remote VaultIds | Valid remote vault | Stop with a vault conflict. Never merge keyrings or select a side automatically. |
| Any local state | Unsupported or malformed metadata | Leave both sides unchanged and report a recoverable validation error. |
| Same DeviceId, different public key | Any valid remote vault | Stop with an identity conflict. Require explicit reinstall/revocation handling. |

Partial creation is detected by missing or invalid metadata, a missing wrapped
key for every active device, or a revision mismatch. Cleanup must be explicit;
the client does not overwrite an ambiguous remote directory in place.

## First-join preview contract

The provider-independent implementation lives in `internal/vaultmeta`. It
validates `vault.json`, scans the current tracked save folders and named save
locations, carries measured discoveries into a local-only preview (which can
still include tracked locations and must be reviewed),
and compares that preview with a remote library summary. It has no provider
client and exposes no apply/write operation.

The Cloud Backup screen now offers an on-demand, read-only local scan through
`GET /api/cloud/join/local-preview`. It reuses the existing measured discovery
pipeline and shows tracked games, detected locations, and incomplete-scan
warnings. It does not fetch a remote vault, compare libraries, register a
device, or enable joining; those remain separate Phase 5 work.

The same screen also lists recognizable remote snapshot ZIPs using the existing
`GET /api/cloud/browse` inventory for the currently saved provider. A failed
remote listing is shown as unknown, never as empty, and a local scan can still
be reviewed if the provider is unavailable. This inventory has no trusted
content hash, ancestry, `vault.json`, or wrapped-key proof, so matching game
labels/counts must not authorize an upload, restore, or second-device join.

The comparison produces only these relationships:

| Relationship | Proof |
| --- | --- |
| `identical` | The canonical content hashes match. |
| `local-only` / `remote-only` | The GameId exists on only one side. |
| `local-ahead` | The remote head is explicitly present in the local immutable ancestry list. |
| `remote-ahead` | The local head is explicitly present in the remote immutable ancestry list. |
| `conflict` | Content differs and neither direction has provable ancestry. |

`latestAt` is display metadata only. It is never read by the relationship
algorithm. Existing SQLite snapshot order is likewise reported only as a
latest snapshot hint; because legacy rows have no parent links, the scanner
does not invent a head or ancestry from their timestamps.

An unreadable tracked primary or named save location makes the scan fail
closed. An unmeasured discovered candidate remains visible as unknown and
marks the preview incomplete; unknown never means empty. Different local and
remote VaultIds stop at a vault conflict before any per-game action is
proposed. Every create, publish, or join plan requires a separate confirmation,
and each `conflict` row additionally requires an explicit per-game resolution.

## Snapshot ancestry and attribution mapping

The current snapshot engine remains authoritative. Existing data maps as
follows:

| V2.1 logical field | Existing source | Migration decision |
| --- | --- | --- |
| `snapshotId` | `snapshots.id` | Reuse unchanged. |
| `gameId` | `snapshots.game_id` | Reuse unchanged. |
| branch | `snapshots.branch_name` | Reuse unchanged. |
| `createdAt` | `snapshots.timestamp` | Reuse after strict parsing; do not use as ancestry. |
| content hash | File hashes in `snapshot_files`, or the archive when legacy rows lack file records | Compute a canonical tree hash when publishing. Do not reuse `games.last_manifest_hash`, which describes watcher state rather than an immutable snapshot. |
| `deviceId` | Not stored on snapshot rows | A future forward migration adds a nullable field. New snapshots set it from the stable DeviceId; legacy rows stay unknown. |
| `parentSnapshotId` | Not stored on snapshot rows | A future forward migration adds a nullable self-reference. New snapshots record the known branch head. Legacy ancestry remains unknown. |

`game_peer_sync_state.agreed_hash` and `game_root_sync_state.agreed_hash` are
pairwise convergence markers, not snapshot parent links. Branch order and
timestamps also do not prove parentage. Legacy rows must therefore remain
`NULL`/unknown rather than receiving invented parents or device attribution.

The eventual minimal migration should add nullable `device_id`,
`parent_snapshot_id`, and immutable `content_hash` snapshot metadata, with
upgrade tests against existing databases. It is intentionally deferred until
the provider-independent metadata/index contract and legacy fixtures are
approved.

## Destructive-operation safety gate

Before any restore or incoming replacement that would remove or overwrite a
local file:

1. inspect the primary save and every named save location;
2. create a safety snapshot of the complete current game state;
3. require snapshot creation to succeed;
4. only then apply the restore, pull, or keep-remote decision.

An inability to inspect or snapshot the current state is a hard failure. The
operation leaves local files and pending conflicts unchanged. This applies to
manual restore, cloud/backup restore through the snapshot manager, primary P2P
replacement, extra-location P2P replacement, and keep-remote conflict paths.

## Deliberately deferred

- No SQLite schema migration is introduced by this design slice.
- No provider API, OAuth flow, wrapped-key upload, or cloud snapshot index is
  implemented here.
- The first-join scan/summary and confirmation contract are implemented. The
  screen that presents and applies the preview remains Phase 5 work and must
  preserve this explicit-confirmation boundary.
- Vault discovery never authorizes an automatic restore or upload.
