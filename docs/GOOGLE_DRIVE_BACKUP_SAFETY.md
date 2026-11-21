# Google Drive backup safety

Status (2026-11-21): conservative snapshot backup with bounded recovery within
one upload. Automated contract tests use mock HTTP servers. The live checks
below used a dedicated account and two isolated Windows profiles.

## Documented API behavior and current safeguards

- Google's [files.create](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/create)
  creates a file resource; the app does not rely on a same-name uniqueness
  guarantee that this method does not state. An upload preflight lists all
  pages, but another writer may create a same-name object afterward.
- [files.list](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/list)
  provides page tokens and an `incompleteSearch` flag. The app rejects
  incomplete, repeated, or overlong snapshot and auto-folder listings. It
  refuses to guess among multiple root folders named `OpenSave`, and checks
  a newly created folder's ID in a fresh listing. An explicit folder ID in
  Settings bypasses only this auto-folder discovery, not snapshot checks.
- Google's [resumable-upload guide](https://developers.google.com/workspace/drive/api/guides/manage-uploads)
  says interrupted or 5xx chunk requests must query session status before
  resuming; the response `Range` identifies received bytes. The app now does
  this with a bounded number of status checks instead of blindly replaying
  a possibly committed chunk. It requires a verifiable final file response,
  then checks a fresh complete folder listing for exactly one matching name,
  ID, and size. Unknown completion is reported as failure; no remote object
  is automatically deleted or overwritten.
- Each ZIP upload reserves one object ID with [files.generateIds](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/generateIds)
  using the current `drive.file` permission. Session creation can be attempted
  at most twice and always uses the original ID. After an expired session or
  indeterminate result, the client asks Drive for that exact ID before retrying.
  In particular, a create response of `409` can be recovered only by confirming
  the object through [files.get](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/get).
  The ID, name, folder, type, live status, byte count and SHA-256 must match, then
  the original complete-list check still runs. A missing checksum or mismatched
  object stays unconfirmed. The ZIP is hashed from disk using bounded memory.

These checks detect ambiguity but cannot prove that two devices cannot create
the same name concurrently, nor that a post-upload listing is instantly
consistent. The UI therefore labels Google Drive as backup-only. A failed
post-upload check may leave a valid remote object; inspect the folder before
retrying. Do not infer ancestry, Vault identity, or permission to restore from
matching names or sizes.

Overlapping automatic and manual backups for the same name share one in-flight
attempt within a daemon. The later caller receives a name conflict if the
first attempt succeeds, or receives the first attempt's error. Once the attempt
ends, a later manual retry or restart begins a new operation with its own
reserved ID. A persistent queue would need to retain and revalidate the
reserved ID, destination and local content identity across restarts and
account changes.

## Manual release checks (disposable account, synthetic ZIPs only)

The first grant omitted the requested `drive.file` permission and the app
rejected it. Two later attempts expired while awaiting consent. In the final
run both profiles obtained the Drive permission, refreshed credentials and
read account quota metadata. Both profiles were disconnected locally after
the run.

| Check | Result | Evidence and remaining coverage |
| --- | --- | --- |
| Existing unique `OpenSave` folder and snapshot pagination | PASS | Both profiles selected the same folder. Snapshot listing followed multiple real Drive pages. Folder discovery pagination has mock coverage. |
| Duplicate folders and concurrent folder creation | PASS | Both reads refused to choose among duplicate folders after concurrent creation under an isolated parent. |
| Resumable upload interrupted after an accepted chunk | PASS | The test dropped the accepted chunk response and observed a real Drive status-query `Range`. The accepted chunk was sent once. |
| Session expiry or lost response | PASS | Controlled faults around real requests verified that retry reused the reserved ID. A real Drive 409 was recovered through exact-ID and SHA-256 verification. |
| Final response and fresh listing | PASS | Automatic and manual backups in one daemon produced one remote object. A later same-name attempt was refused. |
| Concurrent same-name writes from independent clients | PASS for restore rejection | The race left two same-name files. Restore refused the ambiguous source and preserved the current save. |
| Delayed listing, quota and rate limit | PASS with injected responses | Controlled faults verified that the client reported failure without starting a duplicate upload. Natural session expiry, account exhaustion and service throttling were skipped. |
| Download, verify and restore a synthetic snapshot | PASS | The second profile verified the remote snapshot and restored matching bytes. The displaced current save was verified in the safety snapshot. |

Do not record OAuth tokens, account email, folder IDs, Authorization headers,
remote response bodies, or save contents in test evidence or the repository.
