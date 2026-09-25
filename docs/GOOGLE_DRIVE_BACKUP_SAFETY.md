# Google Drive backup safety

Status (2026-09-25): conservative snapshot backup, **not** verified atomic
multi-device cloud sync. All current contract tests use mock HTTP servers; no
new real-account compatibility result is claimed.

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

These checks detect ambiguity but cannot prove that two devices cannot create
the same name concurrently, nor that a post-upload listing is instantly
consistent. The UI therefore labels Google Drive as backup-only. A failed
post-upload check may leave a valid remote object; inspect the folder before
retrying. Do not infer ancestry, Vault identity, or permission to restore from
matching names or sizes.

## Manual release checks (disposable account, synthetic ZIPs only)

| Check | Result | Evidence needed |
| --- | --- | --- |
| Existing unique `OpenSave` folder and paged folder listing | BLOCKED | Record sanitized page/status sequence and whether the selected ID stays stable; do not record the ID itself. |
| Duplicate root folders and concurrent folder creation | BLOCKED | Confirm the client stops and never silently chooses one. |
| Resumable upload interrupted after an accepted chunk | BLOCKED | Confirm status-query `Range` and no blind replay of already accepted bytes. |
| Final response ID/size and fresh listing | BLOCKED | Confirm requested fields are returned and a unique remote object is visible. |
| Two-device same-name creation | BLOCKED | Confirm duplicate detection and manual recovery; do not claim atomic create-only even if one trial appears to pass. |
| Delayed listing, expired session, quota, and rate limit | BLOCKED | Record controlled failure and ensure no automatic duplicate retry. |

Do not record OAuth tokens, account email, folder IDs, Authorization headers,
remote response bodies, or save contents in test evidence or the repository.
