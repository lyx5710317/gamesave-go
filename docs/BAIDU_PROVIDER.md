# Baidu Netdisk Provider Design (Not Implemented)

Status: design and verification checklist only. No Baidu API code, credential, endpoint assumption, or production account is included in Phase 0/1.

## Goal and boundary

Baidu Netdisk is the planned primary mainland-China provider. It must move encrypted/versioned OpenSave snapshot artifacts directly between the Windows client and the user's Baidu account. A GameSave Cloud service must never receive game-save bytes.

Before implementation, verify all flows, scopes, redirect rules, native-app eligibility, quotas, upload limits, review requirements, and current terms against Baidu's official developer documentation. Values that are not officially confirmed remain `TBD`; reverse-engineered browser/cookie APIs are out of scope.

## Authentication inputs and lifecycle

- `AppKey` identifies the OAuth application and may be public only when Baidu explicitly supports a native public-client model.
- `SecretKey` is confidential and must never be hard-coded, bundled, logged, committed, or delivered to the open-source Windows client.
- `AccessToken` is short-lived authorization, held only as long as necessary and redacted from diagnostics.
- `RefreshToken` renews authorization without asking the user to sign in again and requires protected persistence and rotation handling.
- Authorization should use the system browser and a provider-approved redirect mechanism. PKCE is required when available.
- Device Authorization may be used only if Baidu officially documents it for this client type; polling intervals and expiry must be honored.
- Disconnect must revoke when supported and remove local protected credentials without deleting remote saves.

## Two acceptable OAuth models

### A. Public Client OAuth

Preferred if Baidu officially permits a Windows native public client that does not possess a client secret. Use Authorization Code plus PKCE (or the provider's official equivalent), a loopback/custom-scheme redirect approved for native apps, and direct client-to-provider token refresh.

### B. Minimal OAuth Broker

Fallback only if Baidu requires a confidential `SecretKey`. The broker may perform only OAuth code exchange and token refresh. It must use narrow request validation, short-lived correlation state, rate limits, auditable secret handling, and no user account database beyond what is strictly required for abuse prevention. It must not proxy, inspect, cache, log, or store game-save files.

The broker decision and threat model require a separate security review before implementation.

## Provider operations

An isolated provider contract should cover capability discovery, authentication status, list/stat, upload, download, delete only when explicitly requested, and token refresh. Baidu-specific response/error translation belongs inside the adapter; snapshot, ancestry, conflict, and restore rules remain provider-neutral.

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

### Rate limits and retry

- Classify authentication, quota, throttling, transient transport, integrity, and permanent request errors.
- Honor official retry headers. Otherwise use capped exponential backoff with jitter.
- Pause the queue and surface action for expired consent, exhausted quota, or repeated integrity failure.
- Never turn an uncertain upload result into a duplicate logical snapshot; reconcile by stable remote path and hash first.

## Token storage

Current OpenSave settings can represent OAuth tokens in SQLite, but that is not sufficient for this provider's release bar. On Windows, store refresh credentials with DPAPI or an equivalently reviewed OS-protected credential facility. Keep only non-secret provider/account display metadata in normal SQLite rows. Migration must preserve existing provider data and have upgrade/rollback tests.

## Vault directory

The logical provider application directory is:

```text
/apps/GameSaveCloud/
  vault.json
  snapshots/<gameId>/<snapshotId>...
  manifests/...
```

`vault.json` is versioned and starts with a generated `vaultId`, creation time, and registered devices. Exact Baidu application-directory semantics and path limits are `TBD` pending official verification. Object layout must be provider-neutral at the logical layer even if the adapter maps it to provider-specific IDs.

## Required tests before release

- OAuth success, denial, expiry, refresh rotation, revocation, and clock skew.
- Missing/invalid protected credential and migration from older storage.
- Small, multipart, resumed, duplicated, throttled, quota-exhausted, and interrupted uploads.
- Resumed download, corrupt part, size/hash mismatch, and safe temporary-file cleanup.
- Concurrent devices, idempotent retries, vault discovery, and no silent overwrite.
- Proof that broker logs and traffic never contain save bytes or tokens.

## Open questions / launch gates

- Is a third-party open-source Windows native client eligible for Baidu Netdisk OpenAPI access?
- Is public-client OAuth with PKCE supported, or is a confidential exchange mandatory?
- Which redirect URIs and device-authorization flows are approved?
- What are the current scopes, per-user/application rate limits, file-size/part-size limits, hash semantics, and application-directory restrictions?
- Are background refresh, redistribution, and open-source client IDs permitted by current terms?

No provider implementation begins until these questions have primary-source answers recorded here.
