# Jianguoyun official-WebDAV preset

Status (2026-09-24): near-term mainland-China recommendation for **single-device snapshot backup**, not a production claim of safe multi-device cloud synchronization. All compatibility results below are mock-server results unless explicitly marked otherwise. No real Jianguoyun account or credential was used in automated tests.

## Official evidence checked on 2026-09-24

- The [official WebDAV help collection](https://help.jianguoyun.com/?tag=webdav) explains use of the account's registration email and a separately generated third-party application password. Do **not** use the normal account login password. The documented HTTPS endpoint is `https://dav.jianguoyun.com/dav/`.
- The same help collection states a default WebDAV single-file limit of 500 MB, at most 600 requests per 30 minutes for free accounts and 1,500 for paid accounts, and at most 750 files/folders in one directory response. It says multi-page loading is supported, but does not publish the request parameters, cursor/offset format, completion marker, or consistency semantics needed to implement it safely.
- The [official pricing page](https://www.jianguoyun.com/s/pricing) currently advertises free-plan monthly traffic of 1 GB upload and 3 GB download, with storage influenced by uploads. Paid-plan descriptions differ. Marketing about transfer speed or client upload size must not be applied to this WebDAV integration; the WebDAV-specific 500 MB default is the conservative client limit.

These pages describe the service, not a tested contract for this app. No undocumented or reverse-engineered Jianguoyun API is used.

## Preset and data boundaries

The provider identifier is `jianguoyun`, routed through the existing cloud `Provider` boundary and WebDAV operations. The UI fixes the base URL to the official endpoint; the backend resolves snapshot objects beneath `/dav/GameSaveGo/`, never the drive root. The folder is created with standard `MKCOL` only after a `PROPFIND` check; a concurrent-creation response is rechecked. Existing generic WebDAV, P2P, Google Drive, Dropbox, OneDrive, local-folder, webhook, Baidu design/seam, and Quark candidate status remain intact.

Windows Credential Manager stores the third-party application password under the stable local NodeID and provider name. The SQLite `cloud_config.password` column is empty for this preset and for legacy generic-WebDAV rows pointing at the official Jianguoyun host. An older plaintext row is migrated by writing the OS credential first and clearing the SQL column second; a failed SQL clear restores the prior credential or removes the new one. If protected storage is unavailable or corrupt, operations fail instead of falling back to plaintext. The settings API returns only `passwordConfigured`, not the password. A blank update preserves it; a dedicated disconnect removes it and disables this backup destination without deleting remote files. The registration email remains local configuration metadata and is not logged.

The generic-WebDAV form cannot bypass the preset's upload checks by using the official Jianguoyun hostname. Such a legacy row is masked and migrated on read, but backups must be switched to the dedicated preset before they can be saved or used. This also prevents a plain-HTTP variant from carrying the application password to the service.

## Backup-only safeguards implemented

- Reject a snapshot above the conservative 500,000,000-byte boundary before any WebDAV request.
- Pace one process's requests to the free-tier budget (one request every three seconds); retry bodyless metadata requests at most three times with exponential backoff and bounded `Retry-After` handling. Never blindly retry an uncertain snapshot PUT.
- Classify authentication, permission/possible traffic exhaustion, missing folder/object, precondition collision, rate limit, storage exhaustion, and network interruption without echoing response bodies, email, Authorization headers, or the application password.
- Refuse a directory response of 750 or more entries, malformed XML, duplicate or out-of-folder objects, or an unrecognized direct child of `multistatus` (including a page marker). Until the official pagination contract is supplied, this is a **fail-closed cap**, not completed pagination. A smaller ordinary `Depth: 1` response is treated as a provisional backup inventory, not proof of complete remote ancestry.
- Recheck the remote snapshot name immediately before PUT. Use `If-None-Match: *` and a random harmless probe object (rechecked when this process's destination or credential changes) to check whether the server rejects a second conditional create and preserves the first. Reject a server that ignores the condition or fails to remove the probe. Confirm the real object's `HEAD` length matches the local snapshot size. The probe does **not** prove atomic behavior under concurrent real accounts; it is only a local safety tripwire.
- Existing `DownloadVerified` stages downloads, checks the archive size and ZIP contents/CRC, and publishes without replacing a different local archive. Failed or interrupted downloads do not restore live saves. The snapshot manager's safety-snapshot gate remains mandatory before any restore.

## Not established; release blockers

1. Official pagination protocol and exhaustive inventory beyond 750 objects. Do not infer “remote empty” from a capped or ambiguous response. Interrupted pages, duplicate pages, repeated/cycling cursors, and invalid completion markers require contract tests once the protocol is known.
2. Real-account behavior of `If-None-Match: *`, conditional conflicts, ETag, `HEAD` length, concurrent `MKCOL`, rate-limit responses, `Range`, locks, and server error bodies. A mock server cannot establish these guarantees. Snapshot create-only and `vault.json` compare-and-swap are separate capabilities.
3. Strong ETag/CAS replacement for `vault.json`; the preset does not implement a vault metadata writer. No remote Vault join, device registration, ancestry-aware reconciliation, or production multi-device cloud write is enabled. Timestamps never choose a winner.
4. Multi-process/multi-device aggregate pacing, durable retries, resumable transfers, provider-authenticated remote identity, and recovery after ambiguous PUT outcomes.
5. Windows clean-machine installation, credential migration from a real older profile, and account-specific free/paid limits. Non-Windows protected-password support is deliberately unavailable until reviewed.

## Manual real-account checklist (no credentials in reports)

Use a disposable Jianguoyun test account and non-sensitive synthetic ZIPs. Record only PASS, FAIL, or BLOCKED and diagnostic status categories; never attach the email, application password, request Authorization header, account ID, or save contents.

| Check | Current result | Evidence needed to unblock |
| --- | --- | --- |
| Official URL, separate `GameSaveGo/` folder, `MKCOL`/existing-folder race | BLOCKED — no test account supplied | Capture sanitized method/status sequence and verify the folder contains only test objects. |
| 500 MB exact boundary and over-limit rejection | BLOCKED — no test account supplied | Confirm provider's decimal/binary interpretation; keep the conservative client boundary meanwhile. |
| 401/403/404/412/429/507 and network recovery | BLOCKED — no test account supplied | Record sanitized responses and retry timing on free and paid plans. |
| 750+ items and official pagination | BLOCKED — official page protocol not published | Obtain official parameters and test all pages, interruptions, repetition and cursor loops. |
| Concurrent same-name PUT and ignored conditional header | BLOCKED — no test account supplied | Two-device repeatable test proving one stored object and no overwrite. |
| Post-upload size, ETag/CAS, Range, locks and restore staging | BLOCKED — no test account supplied | Repeatable provider-specific contract and restore tests. |
| `vault.json` conditional update / second-device join | BLOCKED — no strong CAS proof or completed join flow | Separate design and tests; do not enable from snapshot PUT results. |

Mocked automated tests exercise password storage and rollback, settings masking, size guard, status classification, capped/invalid inventories, conditional-header rejection, concurrent same-name creation, bounded retry, length mismatch, and interrupted download. They do not convert any BLOCKED manual result into PASS.
