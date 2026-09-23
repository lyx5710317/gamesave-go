# Baidu Netdisk Provider Design (Not Implemented)

Status (2026-09-23): official API documentation reviewed; no Baidu adapter, credential, OAuth broker, or production account is included. Public app approval and the security review remain launch gates.

## Goal and boundary

Baidu Netdisk is the planned primary mainland-China provider. It must move encrypted/versioned OpenSave snapshot artifacts directly between the Windows client and the user's Baidu account. A GameSave Cloud service must never receive game-save bytes.

The official documentation now describes a software-application path, OAuth authorization, application-directory restrictions, quotas, upload limits, and public-release review. It does **not** establish that our open-source Windows app has been approved or that public-client OAuth is supported. Reverse-engineered browser/cookie APIs remain out of scope.

## Official-source verification (2026-09-23)

The following is documentation evidence, not a grant of production API access:

| Topic | Verified documentation | Remaining gate |
| --- | --- | --- |
| Application eligibility | [Create application](https://pan.baidu.com/union/doc/使用入门/创建应用.md) includes an OS software application type. Unreviewed personal/test apps have a 10-user limit. Public distribution requires online review. | Register the project application and obtain public-release approval under the actual distribution model. |
| Scope and OAuth | [Authorization-code mode](https://pan.baidu.com/union/doc/使用入门/接入授权/授权码模式.md) documents `basic,netdisk` scope and requires `SecretKey` for code exchange and refresh. Code lifetime is 10 minutes; access tokens are documented as valid for 30 days; refresh tokens rotate on use. | Confirm the exact approved redirect and broker integration for this application. Do not ship `SecretKey` in the client. |
| Device code | [Device-code mode](https://pan.baidu.com/union/doc/使用入门/接入授权/设备码模式授权.md) also requires `SecretKey` for token exchange and refresh, and polling no more often than every 5 seconds. | Device-code mode does not remove the confidential-client requirement. |
| Redirect | [Callback address](https://pan.baidu.com/union/doc/使用入门/接入授权/授权回调地址.md) allows configured callbacks and documents `oob`. | Obtain explicit confirmation for the chosen redirect; loopback/custom-scheme support has not been established. |
| Quota and directory | [Permissions and quotas](https://pan.baidu.com/union/doc/使用入门/权限与配额.md) describes 10 calls/hour and 10 users before review, then approved API/frequency permissions. Default file access is under `/apps/{appname}`. Applications created after 2026-06-03 have additional application-directory restrictions in the [create application](https://pan.baidu.com/union/doc/使用入门/创建应用.md) document. | Confirm granted APIs, rate limits, application directory name, and any partner-specific conditions after review. |
| Upload | [Upload capability](https://pan.baidu.com/union/doc/基础网盘服务/上传/能力说明.md) and [part upload](https://pan.baidu.com/union/doc/基础网盘服务/上传/分片上传.md) document pre-create, part upload, commit, and provider-specific size/part limits. Ordinary accounts are documented with a 4 GB file limit and up to 1024 parts. | Confirm the upload-host discovery and checksum semantics for the granted APIs before coding retries/resume. |
| Review | [Application online review](https://pan.baidu.com/union/doc/使用入门/应用上线审核/应用上线审核试运行.md) describes public-release review materials and demonstration requirements. | Complete review; do not distribute a test-only integration as production. |

Decision: **minimal OAuth broker**, subject to application approval and a separate security review. The published authorization-code and device-code flows require a confidential `SecretKey` for exchange and refresh; no approved public-client/PKCE route was found in the reviewed documentation. This is a decision from the documented flows, not a claim that Baidu forbids every possible public-client arrangement. The broker is limited to OAuth code exchange and refresh; save bytes must travel directly between the client and Baidu.

## Authentication inputs and lifecycle

- `AppKey` identifies the OAuth application; its exposure in a distributed client still requires provider approval.
- `SecretKey` is confidential and must never be hard-coded, bundled, logged, committed, or delivered to the open-source Windows client.
- `AccessToken` is short-lived authorization, held only as long as necessary and redacted from diagnostics.
- `RefreshToken` renews authorization without asking the user to sign in again and requires protected persistence and rotation handling.
- Authorization should use the system browser and a provider-approved redirect mechanism. Use PKCE if Baidu confirms support for this application flow.
- Device Authorization may be used only if Baidu officially documents it for this client type; polling intervals and expiry must be honored.
- Disconnect must revoke when supported and remove local protected credentials without deleting remote saves.

## Two acceptable OAuth models

### A. Public Client OAuth

Preferred if Baidu officially permits a Windows native public client that does not possess a client secret. Use Authorization Code plus PKCE (or the provider's official equivalent), a loopback/custom-scheme redirect approved for native apps, and direct client-to-provider token refresh.

### B. Minimal OAuth Broker

Fallback only if Baidu requires a confidential `SecretKey`. The broker may perform only OAuth code exchange and token refresh. It must use narrow request validation, short-lived correlation state, rate limits, auditable secret handling, and no user account database beyond what is strictly required for abuse prevention. It must not proxy, inspect, cache, log, or store game-save files.

The broker threat model and implementation require a separate security review. The current provider boundary does not implement any OAuth flow.

## Provider operations

The first transport seam is `internal/cloud.Provider`: upload, list, download, and explicit delete route through a registry. Existing providers remain behind a compatibility adapter, so their behavior is unchanged. The Baidu adapter must extend this seam for capability discovery, authentication status, remote stat, conditional metadata writes, and token refresh without reimplementing snapshot, ancestry, conflict, or restore rules. The current seam is **not** sufficient by itself to publish Baidu sync.

### Upload

- Create the vault directory and deterministic snapshot object paths without exposing local absolute paths.
- Use the official pre-create/part-upload/commit sequence for files above the documented threshold.
- Persist a resumable upload record containing only provider upload ID, part completion, object identity, content size/hash, attempt count, and expiry.
- Verify the committed remote size/hash using officially supported fields; never mark a queue item complete from an HTTP success alone.
- Bound concurrency and make retry idempotent.

### Download

- Download to a temporary file, support HTTP range/resume only when officially supported, and verify size plus local cryptographic hash.
- Never extract or restore directly from the network stream.
- Pass the verified artifact through the existing snapshot/restore safety path.

The existing cloud-restore route now uses `cloud.Service.DownloadVerified`: it stages the file next to the local backup, checks provider-reported size when available, reads every ZIP entry to verify its CRC, rejects unsafe archive paths, and publishes only after verification. A different local archive with the same snapshot identity is preserved and reported as a conflict (HTTP 409). Interrupted, truncated, corrupt, and path-traversal cases have unit and end-to-end coverage. Existing providers do **not** supply a trusted SHA-256; the helper accepts one for a future authenticated vault manifest, but a hash calculated only after download is not proof of remote authenticity. This work is not Baidu transfer or resume support.

### Rate limits and retry

- Classify authentication, quota, throttling, transient transport, integrity, and permanent request errors.
- Honor official retry headers. Otherwise use capped exponential backoff with jitter.
- Pause the queue and surface action for expired consent, exhausted quota, or repeated integrity failure.
- Never turn an uncertain upload result into a duplicate logical snapshot; reconcile by stable remote path and hash first.

## Token storage

Current OpenSave settings can represent OAuth tokens in SQLite, but that is not sufficient for this provider's release bar. On Windows, store refresh credentials with DPAPI or an equivalently reviewed OS-protected credential facility. Keep only non-secret provider/account display metadata in normal SQLite rows. Migration must preserve existing provider data and have upgrade/rollback tests.

The first protected backend now lives in `internal/cloud/protectedtokens`. On Windows it uses the current user's [Credential Manager generic credentials](https://learn.microsoft.com/en-us/windows/win32/api/wincred/ns-wincred-credentialw), scoped by provider and a caller-supplied stable local installation ID (the existing `NodeID` is the planned input). A token pair is replaced in one [CredWriteW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew) call; reads and deletion use the matching OS APIs. The record is versioned, rejects incomplete/corrupt data, and enforces the documented 2,560-byte credential-blob limit instead of falling back to plaintext. Non-Windows builds report unsupported until an equivalent secure backend exists. Same-user malware can still access a user's credentials; this is not a substitute for an OAuth/broker security review.

This backend is **not yet wired to Baidu OAuth**, because the app registration, approved redirect, broker, and provider adapter are still pending. It does not migrate or change Google, Dropbox, or OneDrive's existing SQLite tokens. No schema migration is needed for this isolated addition; any future migration of existing providers must retain their data and have upgrade/rollback tests.

## Vault directory

The logical provider application directory is:

```text
/apps/GameSaveCloud/
  vault.json
  snapshots/<gameId>/<snapshotId>...
  manifests/...
```

`vault.json` is versioned and starts with a generated `vaultId`, creation time, and registered devices. The official API documents `/apps/{appname}` as the application directory. The displayed `GameSaveCloud` folder above is illustrative until the actual approved application name is fixed; the adapter must derive it from that name. Exact path-length and conditional-write semantics remain `TBD`. Object layout must be provider-neutral at the logical layer even if the adapter maps it to provider-specific IDs.

## Required tests before release

- OAuth success, denial, expiry, refresh rotation, revocation, and clock skew.
- Missing/invalid protected credential and migration from older storage.
- Small, multipart, resumed, duplicated, throttled, quota-exhausted, and interrupted uploads.
- Resumed download, corrupt part, size/hash mismatch, and safe temporary-file cleanup.
- Concurrent devices, idempotent retries, vault discovery, and no silent overwrite.
- Proof that broker logs and traffic never contain save bytes or tokens.

## Open questions / launch gates

- Will Baidu approve this particular open-source Windows app for public distribution, and with which API whitelist and rate limits?
- Which redirect is approved for the broker flow? Is PKCE additionally supported?
- What are the exact upload-host discovery, hash, stat, and conditional-write guarantees under the granted APIs?
- Are background refresh, redistribution, and open-source client IDs permitted by the final approval terms?

No production Baidu OAuth or file-transfer implementation begins until the required approvals and security review are recorded here. Provider-neutral boundary work and tests may proceed without credentials or API calls.
