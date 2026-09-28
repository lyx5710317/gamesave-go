# Cloud archive ingress and Windows restore checks

Status: automated regression coverage implemented on 2026-09-26. After an
initial VM failure and then host read failure, local inspection found a disabled
host cloud gate. The owner reports **PASS** for both devices browsing after
enabling/saving, plus new VM upload and host verification. The 2026-09-27
follow-up reports explicit cloud restoration with matching content and recovery
after automatic-safety-snapshot instructions. Exact candidate identity and an
independent whole-tree/hash comparison remain open.
Earlier upload/restore results remain evidence for those earlier builds only;
the follow-up sections below distinguish each result and remaining gate.

## Safety boundary

The existing cloud `DownloadVerified` and read-only `VerifyRemoteSnapshot`
share the same archive inspection. After downloading to a temporary file:

1. Compare a known positive listed size before hashing/decompression. Zero
   remains an unknown legacy size, not proof of an empty archive.
2. Inspect the whole ZIP entry tree before reading entry payloads. Refuse
   duplicate explicit paths, non-canonical paths (`.`/`..`/repeated separators),
   file-versus-directory/parent conflicts, special file types and non-empty
   directory entries.
3. On Windows reject case-colliding extraction targets, device names, invalid
   Win32 characters and trailing dots/spaces. The rules follow Microsoft's
   [file naming documentation](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file),
   including extension-qualified and superscript COM/LPT device names. Linux
   retains case-sensitive ordinary names rather than inheriting Windows rules.
4. Hash and check ZIP CRCs using the same opened archive handle. Failure
   diagnostics contain no untrusted ZIP entry names or save bytes.
5. Only a fully accepted archive can pass to existing no-overwrite publication,
   snapshot import and the existing pre-restore safety-snapshot gate. The
   read-only action deletes its temporary download without publishing it.

An unsafe tree returns a fixed diagnostic localized by the Cloud Backup UI.
Neither a CRC-valid archive nor a matching local archive proves vault ancestry.
No provider, schema, snapshot engine, local compatibility path or P2P protocol
is replaced. This check is at cloud ingress, not a claim that every legacy
local archive or P2P payload has this exact validation.

Jianguoyun downloads no longer run directory creation. If the remote directory
vanishes, downloading/verifying fails without `MKCOL`; upload setup still uses
the existing standard WebDAV creation and verification checks.

## Limits not established

- This is not a disk quota, decompression budget, complete filesystem-alias
  proof (including short names/junctions), malware scan or atomic live-save
  restore guarantee. Transport byte limits/resource budgets need a separate
  compatibility review; a size check after download does not bound downloads.
- Different Linux paths may legitimately fail Windows import. Keep the
  original remote archive; do not normalize or silently discard entries to
  make it pass. Malformed archives are not automatically deleted or repaired.
- Jianguoyun pagination, concurrent create-only writes, strong ETag/CAS,
  remote-vault joining and ancestry-aware cloud resolution remain open under
  `docs/JIANGUOYUN_PROVIDER.md` and `docs/VAULT_IDENTITY_V2.md`.

## Automated evidence

Synthetic tests cover accepted regular/empty-directory/multi-location archives,
Linux versus Windows case policy, device names, duplicate and parent conflicts,
special modes, hidden directory bodies, early size mismatch and corrupted CRC.
API tests exercise both verification and restore, checking unchanged save bytes,
unchanged remote archives, no imported snapshot rows and no remaining staging
files. Missing Jianguoyun download directories stay missing.

The local publication race test repeats two concurrent verified downloads with
different and identical bytes. Different archives produce one preserved winner
and one conflict; identical archives are safely reusable, with no staging files
left behind. This Windows local-filesystem result is not a provider race test
or validation of FAT/exFAT/network-file-share fallback behavior.

All credentials and file contents in automation are synthetic. Record final
suite/build outcomes in the handoff; no mock result proves a real account.

### Local validation record (2026-09-26)

Branch: `codex/phase-6-release-preflight`; starting working tree was clean at
`be46da0`. No commit, push, tag or release was created by this work.

| Check | Result |
| --- | --- |
| Existing download/verification cloud + API baseline | PASS |
| Existing Jianguoyun/WebDAV focused baseline | PASS |
| New ambiguous-path regression before the fix | FAIL as expected — old code published each tested unsafe archive |
| New read-only missing-directory regression before the fix | FAIL as expected — old download code created the directory |
| Final focused cloud/API packages (`-count=1`) | PASS |
| New archive/read-only/API regressions (`-count=5`) | PASS |
| Concurrent local publication (`-count=5`, 100 two-writer rounds) | PASS |
| Cloud end-to-end tests (`TestCloud_`, `-count=1`) | PASS — 4.321 seconds |
| `go test ./...` | PASS — first full E2E package run took 602.873 seconds; final rerun after all test edits also passed (E2E cached, cloud package 20.494 seconds) |
| `go vet ./internal/cloud ./internal/api` | PASS — adjacent test fixtures no longer copy mutexes |
| Frontend `npm test` | PASS — 95/95 tests |
| Frontend `npm run build` | PASS |
| Desktop `wails build` | PASS — Windows amd64 portable executable produced |
| `git diff --check` | PASS |
| Real Jianguoyun + VM test on this candidate | FAIL reported by owner afterward — host works, VM verification/upload fail; exact errors and binary identity not yet available |
| NSIS installer, Authenticode and clean-machine upgrade | SKIPPED — portable build only; release validation is separate |

Portable file: `cmd/opensave-app/build/bin/GameSaveGo.exe` (ignored build
output, not a repository source file), 23,079,936 bytes. SHA-256:
`610e9cb5523bf5b1019c478e05b6c89bf3f2633a658ecdc12d7f7256cf68e2be`.
The app's desktop version is `1.1.1`. Windows file-property ProductName and
ProductVersion were blank on this local Wails build; the release workflow's
metadata-stamping/signing steps were not run. Do not treat it as a verified
public package.

Changed sources:

- Cloud transport/inspection: `internal/cloud/cloud.go`,
  `internal/cloud/verified_download.go`.
- Cloud regression tests: `internal/cloud/archive_paths_test.go`,
  `internal/cloud/verified_download_test.go`,
  `internal/cloud/jianguoyun_test.go`.
- API response/regressions: `internal/api/routes_cloud.go`,
  `internal/api/routes_cloud_archive_test.go`.
- UI: `cmd/opensave-app/frontend/src/views/CloudBackup.svelte`,
  `cmd/opensave-app/frontend/src/lib/cloudVerification.js`, its test,
  and `src/locales/en.js` / `src/locales/zh-CN.js` in that frontend.
- Documentation: `README.md`, `SPEC_V2.md`, `TASKS.md`, and this document.

## Owner-run Windows VM round trip

Use only a disposable test game/folder, never a real game save. Keep both app
profiles/backups; do not remove old credentials to test an upgrade.

1. Close the old app on the VM and copy the newly built portable executable
   there. This is a development candidate, not a signed public release.
2. On the sending device, create a test game folder with `slot.txt`, an empty
   `empty.txt`, and a subfolder containing another small text file. Record
   relative paths, lengths and SHA-256 locally (do not send save contents).
3. Track the folder, make a manual snapshot, upload using the existing
   Jianguoyun preset, and note whether the transfer succeeds. Never share the
   registration email/application password.
4. On the other device use an isolated test save path with the same GameId
   mapping, browse that snapshot, run **Verify**, then explicitly restore it.
   If an existing same-name archive differs, keep both copies and stop; do not
   bypass the collision check. Before restore put a different synthetic file
   in the target and confirm a safety snapshot was created successfully.
5. Compare the restored relative file tree, lengths and SHA-256 against step 2,
   including the empty file. Verify that the previous local synthetic state is
   recoverable through the safety snapshot. Record **PASS**, **FAIL**, or
   **BLOCKED**, candidate version/hash and failure category only.

Initial outcome (superseded by the later follow-ups): **FAIL reported by owner — VM verification/upload do not pass,
host works**. Diagnosis and retest remain **BLOCKED** pending fixed-category
failure feedback and binary identity. A future PASS here is a round-trip result,
not proof of atomic concurrent writes or a joined cloud Vault.

Official directory-pagination clarification, two-device race testing,
free/paid-plan status behavior and metadata CAS require their own controlled
checklists. No real account is accessed automatically by this change.

## VM failure follow-up (2026-09-26)

The shared confirmation dialog had hardcoded English defaults. It now uses
active-language fallbacks for its title, primary action and cancellation;
caller-supplied labels remain intact. This fixes `Cancel` in verification and
application-password-removal dialogs without altering either action.

Previously every verification error became the same generic UI message. The
API now returns an allowlisted `cloud_verify_*` code and one fixed message.
The frontend maps only known codes to Chinese/English instructions. Tests
cover authentication, permissions, quota/rate limit, network, absent/ambiguous
objects, incomplete inventory, unsafe ZIP paths, listed-size mismatch, ZIP/CRC
integrity and local filesystem errors. Unknown codes remain generic failures.
Neither raw errors nor account/credential/path/archive-entry details are
displayed. Temporary downloads are removed and existing local archives are
preserved on failure.

These are diagnostics and localization fixes, not evidence that the VM upload
problem has been resolved. No upload protocol, credential protection, condition
probe, no-overwrite guard, CRC check or restore safety gate has been weakened.
The rebuilt diagnostic candidate must be tested on the failing VM.

For the owner-run retest:

1. Close old app/tray processes on both devices and copy only the newly built
   executable; preserve profiles, backup folders and existing remote objects.
2. Confirm the displayed version and compare the executable SHA-256 locally
   if multiple development binaries share the same `1.1.1` label.
3. First verify an existing small synthetic cloud snapshot in the VM. Capture
   the localized failure message, with account identifiers hidden.
4. Upload a **new** synthetic snapshot from the VM and record the transfer
   failure category/safety-check step/HTTP status, if shown. Re-uploading an
   existing name is deliberately not an overwrite operation.
5. If the failure is configuration/authentication, save the third-party app
   password on the VM itself. Credentials are device/user protected; copying
   an executable does not transfer the host's Windows credentials. Do not
   remove credentials or use a login password as a diagnostic shortcut.
6. Keep all copies when size/integrity/unsafe-path checks fail. Do not delete
   remote snapshots, disable validation, allow overwrite, or automatically
   retry an uncertain write to make the test pass.

Only fixed failure messages/statuses and candidate version/hash should be
reported; do not send emails, application passwords, tokens or save contents.

### Follow-up validation and candidate

The existing cloud/API and frontend baselines passed before this follow-up.
The shared-dialog localization regression failed against the old hardcoded
defaults before the fix, then passed after it.

| Check | Result |
| --- | --- |
| Focused `go test ./internal/cloud ./internal/api -count=1` | PASS — cloud 27.341 seconds; API 21.819 seconds |
| Frontend `npm test` | PASS — 104/104 tests |
| Frontend `npm run build` | PASS |
| Desktop `wails build` | PASS — Windows amd64 portable executable |
| `go vet ./internal/cloud ./internal/api` | PASS |
| Full `go test ./...` | PASS — E2E 598.389 seconds; API 21.270 seconds; cloud 27.381 seconds |
| Final `git diff --check` | PASS |
| Real-account failing-VM retest | BLOCKED — diagnostic candidate result not yet received |
| NSIS/signing/upgrade validation | SKIPPED — portable diagnostic build only |

Diagnostic candidate: `cmd/opensave-app/build/bin/GameSaveGo.exe`, 23,086,080
bytes, desktop version `1.1.1`. SHA-256:
`38a1b0c63b8e4ab710f2ae07b8b5f126ad3c35edee96c51b83b7b3eb982c7b77`.
This supersedes the earlier binary above, not its historical test evidence.
No commit, push, tag or release was created; previous in-progress work was
preserved on `codex/phase-6-release-preflight`.

Additional follow-up sources: shared `ConfirmDialog.svelte` / `stores.js`,
`api.js`, paired localization catalogs and cloud verification helpers/UI;
`internal/api/routes_cloud_verify_errors_test.go`,
`frontend/src/lib/apiErrors.test.js` and
`frontend/src/lib/confirmLocalization.test.js` (frontend paths relative to
`cmd/opensave-app/`). `SPEC_V2.md` and `TASKS.md` record the unresolved VM gate.

## Host cloud-read gate follow-up (2026-09-26)

The owner next reported that the host also could not read the cloud. A single
loopback cloud-browse request returned HTTP 502; a settings read confirmed
the global cloud switch was **disabled**, with the official Jianguoyun preset
and protected-password configuration still present and no protected-store
error. Only fixed categories and configuration-presence booleans were printed;
no account identifier, password, snapshot name/content or raw error was output.
No settings or credentials were changed and no enabled cloud operation was
performed by this diagnosis. This establishes the host's local gate failure,
not why the switch was off or the VM's failure cause.

The old Settings checkbox described only automatic mirroring, while the
backend's existing gate blocks all manual reads/uploads/restores too. Cloud
Backup also displayed a configured card with no visible global switch, and
the browser's error disappeared into a generic empty window after its toast.

The fix preserves the existing semantics without adding a second setting:

- An exported disabled sentinel retains the existing error text, so the API
  can distinguish a local gate from configuration/authentication/network
  errors. Snapshot auto-upload suppression remains unchanged.
- Settings describes the all-cloud gate and its automatic-upload side effect.
  Cloud Backup exposes the same draft field with an explicit Save settings
  action and disabled status. No switch or credential is silently changed;
  hidden-provider selections stay intact and can use Settings → Sync.
- Inventory endpoints return a fixed `cloud_read_*` code and generic message,
  never raw provider/credential/path/response data. Known codes map to paired
  Chinese/English instructions; unknown codes fail closed.
- Browser errors remain visible in the modal. Failed refresh clears prior
  actionable inventory, not remote data. Failed/ambiguous/incomplete listings
  do not become a known-empty success. Duplicate in-flight refreshes stop at
  the UI boundary. Existing provider safety checks remain in force.

To retest, enable cloud backup and save in **each installation separately**,
then browse once. This also enables automatic uploads of new snapshots.
Existing application passwords do not need removal/re-entry for a disabled
gate. If an enabled installation reports another failure, preserve all copies
and capture only the fixed category. The owner has now confirmed **PASS for
cloud browsing on both host and VM after enabling and saving**. This is
owner-reported browsing evidence, not a full transfer/restore or strong-CAS
PASS. The owner subsequently reported **PASS for a new synthetic VM snapshot
upload and verification on the host**. Exact executable identity was not
supplied; these are owner-reported workflow results, not independent testing of
the rebuilt artifact. Restored-file comparison and pre-restore safety-snapshot
recovery for this run remain open.

| Check | Result |
| --- | --- |
| Focused existing Jianguoyun/cloud-read/verification baseline | PASS — cloud 8.648 seconds; API 1.900 seconds |
| Existing frontend baseline | PASS — 104/104 |
| Focused final cloud/API packages (`-count=1`) | PASS — cloud 26.286 seconds; API 20.104 seconds |
| Final frontend `npm test` | PASS — 108/108 |
| Frontend `npm run build` | PASS |
| Desktop `wails build` | PASS — Windows amd64 portable |
| `go vet ./internal/cloud ./internal/api` | PASS |
| Full `go test ./...` | PASS — E2E 601.897 seconds; API 21.309 seconds; cloud 27.102 seconds |
| Final `git diff --check` | PASS |
| Owner re-enable/read on host and VM | PASS — owner reports both can browse after enabling/saving |
| New synthetic VM upload and host verification | PASS — owner reports both steps succeeded |
| Restored-file comparison, safety-snapshot recovery and exact candidate identity | BLOCKED — these outcomes/identity not yet received |
| NSIS/signature/installed upgrade | SKIPPED — diagnostic portable only |

Portable diagnostic candidate: 23,093,248 bytes; desktop version `1.1.1`;
SHA-256 `c7e2b129a684827e80cb02642505b4cd7f23b44cb1e56f8b3b5f82fb7e403d6a`.
It supersedes the prior diagnostic executable without superseding historical
test evidence. Additional sources include `internal/api/routes_cloud_read_errors_test.go`,
`cmd/opensave-app/frontend/src/lib/cloudRead.test.js` and `Settings` catalog copy.
All code changes are uncommitted on `codex/phase-6-release-preflight`; earlier
work is preserved and no commit, push, tag or release is authorized here.

### Simplified owner recovery exercise

The owner requested step-by-step instructions rather than marking recovery
complete. Use the already shared **test game only**:

1. Record the successfully uploaded VM snapshot ID, then fully exit the VM
   app/tray so P2P cannot change the comparison files during this exercise.
2. On the host select that same test game and use **Open folder**. Stop if it
   points at real saves or the test game is absent; do not substitute a game.
3. Change one existing synthetic text file to a distinctive local test value.
4. Use **Snapshots → Snapshot now**, with a distinctive pre-test comment.
   Keep this manual backup as an extra fallback.
5. Browse cloud snapshots and restore the exact ID from step 1, not whichever
   object has the latest timestamp. Compare the test files with the VM copy.
6. Restore the manual snapshot from step 4 and check that the distinctive
   local test value returns.

This exercise would establish restore and recovery from a known manual backup.
It does **not**, by itself, verify that the automatically created pre-restore
safety snapshot was independently found/recovered. That separate real-device
check, exact binary identity, stronger provider conditions and cloud-vault
ancestry remain release gates. At that point no owner recovery PASS had been
received; see the subsequent 2026-09-27 clarification below.

## Owner recovery clarification (2026-09-27)

The owner clarified that the absent-file report followed restoring a local
snapshot, not the cloud snapshot. Read-only local inspection found the mapped
test directory empty and its current local ZIP with zero entries; the separate
cloud snapshot passed read-only verification without a same-name local copy.
No live save, credential or cloud object was changed by diagnosis. The owner
then reports successful **explicit cloud restore** and **matching contents**.
After step-by-step instructions to modify a disposable local file, restore the
cloud version, then restore the automatically created pre-rollback safety
snapshot, the owner reports **recovery succeeded**. These are owner-reported
workflow PASS results. No independently recorded file-tree/hash comparison,
explicit final test-marker confirmation or two-device candidate hash was
provided, so do not mark those stricter release checks complete.

Eight owner-approved obsolete build binaries were moved to the ignored
`cmd/opensave-app/build/bin/archive/2026-09-27/` directory and SHA-256 checked
after the move. They remain recoverable. No source, credential, profile, cloud
object or save was deleted; the active executable was retained.

## Granular restore safety follow-up (2026-09-27)

### Demonstrated necessity and scope

Review of the existing single-file API revealed that it logged a failed safety
snapshot and continued to write. It also used the full archive entry path in an
extra location rather than the already resolved location-relative path. The
safety snapshot's retention could remove the selected archive before extraction,
and direct extraction truncated live files before CRC failure was known.
New synthetic regressions failed on all these old behaviors before the fix.
This work changes the existing API/extraction boundary only; no protected
snapshot engine, store schema, provider or P2P implementation is rewritten.

### Replacement gate

1. Resolve the configured root and location-relative destination; reject unsafe
   paths and detected linked ancestors. Unknown missing save paths remain
   directory destinations rather than inferring file mode from one ZIP entry.
2. Stage the one selected regular file in OS temp storage, rejecting missing or
   duplicate matches (including Windows case aliases), directories and symlinks.
   Consume the complete entry for CRC/length checks, then flush and close it.
   Retention no longer needs to preserve this source ZIP to finish the operation.
3. Hash an existing destination, create the safety snapshot using the existing
   manager, and reopen the corresponding safety entry to verify CRC and matching
   old bytes. Failure stops before live-file replacement. A successful snapshot
   row alone is not sufficient evidence that the selected file was captured.
4. Recheck the current destination's existence/hash and detected linked path
   components. An observed change stops the operation rather than overwriting it.
5. Copy the staged data into a flushed, closed sibling temporary file, then
   rename into place. CRC, staging and safety failures do not truncate the live
   target; failed publication retains the safety snapshot and removes temp files.

Fixed `file_restore_*` API categories map to paired English/Chinese instructions;
no new diagnostic logs or responses contain save bytes or raw paths. A local
snapshot-list hint explains cloud restoration separately, including empty states.
Known generated whole-restore and single-file safety comments also display in
the selected UI language. Stored comments and arbitrary user comments are not
rewritten; tests cover both generated patterns, both locales and preservation.

### Remaining limits

The last digest/path check is not an OS lock or compare-and-swap against every
possible writer. Do not claim this closes the final check-to-rename race,
proves every junction/short-name/network-filesystem alias, bounds decompression
resources, or makes a multi-file/multi-location restore transactional. Wider
legacy archive-tree validation and whole-directory destination-kind handling
remain separate work. Close the game and other sync writers during manual
recovery. No real account or live-save restore is used by automated tests.

### Current validation

| Check | Result |
| --- | --- |
| Existing restore API/snapshot baseline | PASS — API 6.525 s, snapshot 8.773 s |
| Existing frontend baseline | PASS — 108/108 |
| New regressions against the old implementation | FAIL as expected — safety failure still wrote, location prefix misplaced files, retention lost the source, CRC/duplicate/special entries replaced live bytes |
| Focused final API/snapshot packages (`-count=1`) | PASS — API 23.931 s, snapshot 11.674 s |
| Expanded granular/path tests (`-count=5`) | PASS — final 7.657 s. The unnamed `#00` empty fixture initially produced a database-ID collision; naming it fixed test isolation. The last Windows rooted-path regression first FAILED, then passed after explicit leading-slash rejection. Neither failed run is counted as PASS. |
| `go vet ./internal/api` | PASS |
| Frontend `npm test` | PASS — final 111/111 |
| Frontend `npm run build` | PASS |
| Desktop `wails build` | PASS — final Windows amd64 portable, 23.24 s |
| Default `go test ./...` | FAIL — E2E reached the default 10-minute alarm as `TestSingleFileGameChain` began (0 s reported for that test); E2E package 616.886 s including setup. Other packages returned PASS. Do not call the default invocation successful. |
| Isolated `TestSingleFileGameChain` (`-count=1 -timeout=2m`) | PASS — E2E package 9.935 s including CLI build |
| Intermediate `go test ./... -timeout=20m` | PASS — E2E 613.292 s, before the final leading-slash guard and added path regression |
| Final `go test ./... -timeout=20m` | PASS — E2E 599.165 s, API 20.365 s after the final path guard; no tests removed or skipped |
| Redundant post-record `go test ./... -timeout=20m` invocation | SKIPPED — interrupted after the completed final suite was recorded; no subsequent backend change. This interrupted invocation is not counted as PASS. Final UI/catalog changes were separately tested (111/111) and rebuilt. |
| `git diff --check` | PASS — final source/documentation check |
| New candidate's owner-run granular restore | PASS reported by owner — subsequent `game2` test succeeded after the optional exercise; exact candidate identity and file-tree measurements not independently supplied |
| NSIS/signature/installed upgrade | SKIPPED — portable development build only, no signing/installer workflow executed |

Portable candidate: 23,108,608 bytes, desktop version `1.1.1` development,
SHA-256 `91b764a593738f5f82c05177f3dd6835dae06002066f7dd0518db017b59bfc0b`.
File: `cmd/opensave-app/build/bin/GameSaveGo.exe`. The current process is not
automatically stopped/restarted by this handoff; fully exit its tray instance
before launching the rebuilt file. No commit, push, tag or public release.

Expanded synthetic tests also cover zero-byte regular files, a configured
single-file target with a different archive basename, invalid safety archives,
an observed concurrent edit and failed publication preserving the destination
without leaked sibling staging files. Wider URI/path escaping in the store
needs a separate review after the unnamed temporary fixture collision; this
follow-up makes no store implementation change.

Optional manual granular check, on a disposable text-file game only:

1. Exit old app/tray instances and launch the candidate; record its file hash.
2. Write a distinctive synthetic value and take a manual local snapshot.
3. Change that file and another test file to different values.
4. In **Snapshots → Browse files**, restore only the first file. Confirm its
   snapshot value returns and the second file is unchanged.
5. Find the automatically created single-file safety snapshot and verify the
   changed first-file value remains recoverable. Never use real game saves,
   bypass errors or delete the safety snapshots to finish this check.

## Whole-restore preflight follow-up (2026-09-27)

### Demonstrated necessity

Synthetic regressions failed on the previous implementation: corrupt ZIP
payloads replaced/truncated current files after the directory was cleared;
unmapped archived extra locations were omitted while the manager returned
success; a missing one-file directory extracted into its parent; an existing
single-file target with a different archive basename lost the configured file;
multiple entries over a file target scattered sibling files. The fix extends
the existing Manager.Restore/UnzipTo boundary, not a parallel restore engine.
No schema, archive layout, peer identity or provider implementation is changed.

### Gate and limitations in the 2026-09-27 build

- Always preserve the selected archive in a private temporary ZIP, including
  empty destinations, and consume every entry for CRC/length validation before
  clearing. Reject special entries and directory bodies. Temporary copies are
  cleaned on success/failure; retention cannot remove the validated source.
- Stop before replacement on missing archived-root mappings, overlapping root
  paths and incompatible target kinds. Existing extra-file roots are blocked
  because the legacy multi-root extractor supports directories there. Recheck
  saved mappings after the safety hook. Lower-level UnzipRoots still reports
  unplaced locations for its existing callers; Manager.Restore may not accept
  them as a complete successful restore.
- At that point, creating a safety snapshot when current content existed and
  reading its ZIP fully detected unreadable/corrupt ZIPs, but not omitted
  current files. The 2026-09-28 follow-up below adds the missing comparison.
- Missing destination paths are directories. Existing primary file targets
  require exactly one top-level regular entry and use the configured basename.
  Do not guess a deleted file from one archive entry; a compatible explicit
  destination-kind design remains open.
- Fixed `restore_archive`, `restore_location` and `restore_safety` responses
  have Chinese/English instructions in local and cloud restore screens. New
  categories do not expose raw filesystem/archive/account details.

No complete legacy archive path-alias validator, OS writer lock, decompression
budget, cross-root transaction or automatic mid-write disk-failure rollback is
claimed. Disk failure during the existing extraction can still leave a partial
restore; keep the safety snapshot and close other writers. Cloud ingress retains
its stronger tree validation. No real cloud credentials or user saves were used
in automation; no live restore was performed by this development turn.

The overlap fixture initially FAILED at setup because existing store insertion
already refuses overlapping roots. It now directly tests the restore preflight
boundary with a synthetic mapping; no store validation was relaxed. A new API
test initially failed to compile due to treating its response map as bytes;
the test now decodes the fixed code correctly. These failed runs are not PASS.

### Validation and candidate

| Check | Result |
| --- | --- |
| Existing focused restore baseline | PASS — snapshot 1.354 s, API 2.111 s (`-run 'Restore\|Unzip' -count=1`) |
| New regressions against the old implementation | FAIL as expected — corrupt payload changed current bytes, unmapped location reported success, missing-folder/file-name mapping failed, multiple entries accepted for a file target |
| Final focused packages (`-count=1`) | PASS — snapshot 10.110 s; API 17.058 s after per-game import failure codes |
| New snapshot/API regressions (`-count=5`) | PASS — snapshot 3.620 s, API 0.955 s |
| `go vet ./internal/snapshot ./internal/api` | PASS |
| Frontend `npm test` | PASS — 112/112 |
| Frontend `npm run build` | PASS |
| Desktop `wails build` | PASS — final Windows amd64 portable, 25.628 s after import warnings; initial build 35.211 s |
| Extra cloud subset during full-suite execution | FAIL — one Jianguoyun fixture could not load its synthetic protected password; cause not established, no provider/security check relaxed |
| Isolated failing Jianguoyun test (`-count=3`) | PASS — 0.962 s; does not erase the earlier failure or prove its cause |
| Sequential full cloud retest (`-count=1`) | PASS — 20.731 s; isolated/synthetic only, the prior credential-load failure remains unexplained |
| Default `go test ./...` | FAIL — E2E 616.776 s, two old import-journey assertions expected primary-only success with an unmapped extra location; then the default 10-minute alarm interrupted `TestSyncContentIntegrity`. Other packages passed. |
| Updated two import-journey regressions | PASS — 16.266 s; now assert fixed missing-location failure and unchanged current saves, then explicit mapping and a complete successful restore including excluded files. Assertions were expanded to the new fail-closed contract, not removed or bypassed. |
| Final `go test ./... -timeout=20m` | PASS — E2E 601.318 s, API 19.358 s, cloud 28.373 s, snapshot 9.610 s; all packages passed or had no test files, no tests excluded. The default invocation remains FAIL as recorded above. |
| Final `git diff --check` / worktree review | PASS — development branch retained, earlier dirty changes preserved, no commit/push/tag or generated-runtime diff |
| This new candidate's manual VM/host round trip | BLOCKED — not yet tested; earlier `game2` owner PASS belongs to the prior exercise |
| Installer/signature/installed upgrade | SKIPPED — portable development build only |

Current portable candidate: desktop `1.1.1` development, 23,120,896 bytes,
SHA-256 `12275a97da1941854a437381e15d925ad9711253cc28cb853dfe58287778e64d`.
Path: `cmd/opensave-app/build/bin/GameSaveGo.exe`. No automatic process restart,
commit, push, tag, installer or public release. Previous dirty work and all
provider implementations were preserved on `codex/phase-6-release-preflight`.

Backup-file imports for **already tracked** games now include an allowlisted
per-game preflight code and show the corresponding localized instruction instead
of only an aggregate skipped count. The inner ZIP remains in local snapshot
history for recovery after mapping. The old backup-import journey tests were
expanded to require no partial write before mapping and successful complete
restoration after explicit mapping, including excluded-file contents and the
existing subsequent P2P checks. Untracked backup imports and branch checkout
have separate legacy extraction paths; this manager fix does not certify them.

Files changed for this follow-up: `internal/snapshot/snapshot.go`, `zip.go`,
new `restore_preflight.go` and `restore_preflight_test.go`; API `routes.go`,
`routes_backup.go`, `routes_cloud.go`, new `routes_restore_preflight_test.go`;
frontend `snapshotRestore.js`, `snapshotRestore.test.js`, `GameDetail.svelte`,
`CloudBackup.svelte`, English/Chinese catalogs; `e2e/journeys_test.go` and
`e2e/multiroot_test.go`; `README.md`, `SPEC_V2.md`, `TASKS.md` and this record.
Earlier in-progress files are not new work in this batch and remain intact.

### Whole current-tree safety capture follow-up (2026-09-28)

The previous ZIP-read check was insufficient: a ZIP can be structurally valid
and still omit an empty file, an excluded file, a mapped extra location or an
empty directory, or contain stale bytes. New synthetic regressions first
demonstrated that the prior gate accepted these cases and proceeded to replace
the current save. The existing `Manager.Restore` boundary now:

1. Reads every current regular file before the safety snapshot, recording its
   relative archive name, size and SHA-256. It includes excluded/dot files,
   empty files and directories, and mapped extra roots. An unreadable or
   special entry stops the restore rather than permitting a partial capture.
2. Creates a safety snapshot when there is observed content and fully reads its
   ZIP. An exact entry/content comparison must succeed; a readable but partial,
   stale, duplicated or unexpected ZIP cannot authorize replacement.
3. Rechecks configured locations and scans the current tree again immediately
   before extraction. An observed edit, addition, deletion or new directory
   stops with `restore_changed`; creation/capture failures use `restore_safety`.
   Both have fixed Chinese/English instructions without local paths or content.

This verifies a sequence of observed states, not a globally atomic filesystem
view or an OS writer lock. A writer can still act after the final scan, and an
I/O failure during the existing multi-root extraction can still leave a partial
restore. Legacy path-alias validation, transactional publication and the
separate untracked-import/branch-checkout paths remain open. Do not treat this
as production-grade cloud-vault joining or ancestry-aware conflict resolution.

| 2026-09-28 check | Actual result |
| --- | --- |
| Existing focused restore baseline before the change | PASS — snapshot 2.188 s, API 2.335 s |
| New incomplete-safety and changed-current-tree regressions against the prior gate | FAIL as expected — a readable partial/stale safety ZIP and observed edits/additions/deletions were accepted or overwritten |
| Final focused `go test ./internal/snapshot ./internal/api -run 'Restore\|Rollback' -count=1` | PASS — snapshot 4.469 s, API 3.350 s |
| `go vet ./internal/snapshot ./internal/api` | PASS |
| Final `go test ./... -timeout=20m` | PASS — E2E 602.725 s, API 27.902 s, cloud 36.941 s, snapshot 11.986 s; all other tested packages passed or had no tests |
| Frontend `npm test` / `npm run build` | PASS — 112/112 tests; production assets built |
| Desktop `wails build` | PASS — Windows amd64 portable in 22.291 s |
| Real VM/host restore and snapshot synchronization | PASS reported by owner on 2026-09-28 — restoration succeeded and snapshots synchronized on both devices. Exact executable hash on each device, independent full-tree/size/SHA-256 comparison and failure recovery under concurrent writes were not supplied. |
| Installer/signature/installed upgrade | SKIPPED — only the portable development binary was built |

Portable development candidate: `cmd/opensave-app/build/bin/GameSaveGo.exe`,
23,132,672 bytes, SHA-256
`FB09BFA7FF910C3C6A5607234FC1073805A1BAF46C9FC679E33D76ED492D548F`.
It was not started, installed, signed, committed, pushed or released by this
follow-up. The existing provider code, cloud credentials and user saves were
not modified; automated fixtures were synthetic.

After handoff, the owner reports a successful restore and that snapshots can
synchronize between the two devices. This closes the pending manual happy-path
report for the new safety-capture build, but does not independently establish
that both devices ran the exact candidate bytes, prove cloud-vault ancestry or
validate provider ETag/CAS and concurrent create-only behavior. Those release
gates remain open.

### Owner follow-up and source upload authorization

After the whole-directory/safety-recovery instructions, the owner reports
`game2` testing succeeded and explicitly requests a GitHub source upload.
Record the manual exercise as owner-reported **PASS**, not independently measured
VM/host executable identity, full-tree SHA-256 evidence or provider concurrency
validation. Those stricter release checks remain open; the BLOCKED table row
above records the handoff state before this follow-up.

No implementation changed for this source-upload step. The completed final Go
suite (`-timeout=20m`), 112 frontend tests, frontend build and Wails build above
are reused as evidence for the unchanged code, rather than claiming another run.
Upload the development branch only; this authorization does not publish a tag,
installer, binary release or pull request, and does not mark Phase 5 complete.
