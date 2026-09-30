# GameSave Go 1.1 release preparation

Status: preparation only. No public installer, attested Release, or validated
clean-machine upgrade is claimed by this document. Untracked v2 backup-file
imports now fail closed rather than overwriting a path supplied by the archive;
tracked-game imports still use the verified restore path.

On 2026-09-29, a release-finalization branch added Windows protected storage
for generic WebDAV passwords, custom OAuth client secrets, and custom request
headers. Existing plaintext SQLite values migrate with rollback on failure;
the settings API returns only configured flags. This branch is not yet an
installed VM candidate or a public release. The Linux/Steam Deck storage path
still uses its previous behavior, so the Windows guarantee must not be
generalized to every platform. Changing a generic WebDAV URL cannot reuse the
old URL-bound credential, but cleanup of old credential entries remains under
review.

The first PR #2 Linux race run found a shutdown race in background conflict
resolution and an intermittent two-device deletion-soak failure. The API
shutdown now drains its own background conflict task before closing the daemon;
focused tests passed locally, but a new Linux race run must verify the change.
The deletion failure remains under investigation and blocks a public release.
The subsequent review found and corrected a definite false-success path:
LAN/WAN delete handlers previously ignored disk deletion errors, and the sync
engine ignored transport deletion errors while advancing state. WAN deletion
now also resolves the requested named save location. These corrections have
focused tests, but whether they fully explain the intermittent soak failure
remains unproven until repeated race and two-device runs pass.
Candidate run 36575850861 was attested and its Windows installer independently
downloaded and checked (SHA-256
`5A961EFF006949FCAE83D55A9082994D0CA328CEE8DAA13F4796E621C41560C0`),
but it predates the shutdown fix and is **superseded**. Do not use it as the
final VM candidate.

The branch candidate [run 36589803878](https://github.com/lyx5710317/gamesave-go/actions/runs/36589803878),
built from `31f447fae570f35faac1a68884ab6b8966ab2763`. Its Windows
build and independent verification jobs passed. The downloaded installer is
`GameSaveGo.Setup.exe`, 12,458,199 bytes, SHA-256
`7AE2490E1FAB297584B2EE9FAEDAE99FA97A71BE6862B4215062E30F8EB4A498`;
the locally computed hash matches the verification job log, and local
`gh attestation verify` with the source ref, commit and workflow constraints
passed. Windows metadata shows GameSave Go 1.1.1 and `NotSigned` as expected.
This is **not** a public Release or an installed-VM PASS. On 2026-09-30, the
owner's VM screenshot showed a fully black app window and English tray text
after launching this candidate. Treat the installed-VM startup check as FAIL
and this candidate as **superseded**. The frontend entry point still used
Svelte 4's `new App(...)` class API after the Svelte 5 dependency upgrade;
Svelte 5 removed that API. A regression test reproduced the outdated entry
point, and the entry now uses `mount(App, ...)`. The black window is consistent
with this startup failure; the screenshot does not prove a firewall block.
The tray only receives the selected locale from the mounted UI, so English
on this failed launch is also consistent with the same cause. A newly built
candidate must be installed and checked on the VM before either behavior is
claimed fixed there.
On this commit, two Linux and two Windows race CI jobs passed, and local
`go test ./... -p 2 -timeout 2700s`, 117 frontend tests, frontend build,
Wails build and full `npm audit` passed. The default-timeout `go test ./...`
had earlier hit its 10-minute ceiling, and one earlier concurrent full run
failed a Jianguoyun fixture credential read; the isolated case passed 20
repetitions and the whole cloud package passed 5 repetitions. These failures
remain in the evidence record rather than being rewritten as passes.

The replacement branch candidate is [run 36663430295](https://github.com/lyx5710317/gamesave-go/actions/runs/36663430295)
from commit `2f74f348429f21ba58a5703c3e8d0499d169a001`. Windows build,
attestation, and independent downloaded-byte verification passed; public
release jobs were skipped. Its 12,460,118-byte `GameSaveGo.Setup.exe` has
SHA-256 `694F9F0D1AEACC5134C263D6B9BE66EF552780D75218A6238C6BAC1919760E0F`.
The local download matched both the workflow's artifact ZIP digest and
installer digest. Local GitHub attestation verification constrained to this
repository, branch ref, commit and release workflow passed; Windows metadata
shows GameSave Go 1.1.1 and the expected unsigned status. The source-tree
regression test first failed on the old constructor call and passed after
the `mount` change. Local checks: frontend `npm test` PASS (20 files,
118 tests); frontend `npm run build` PASS; `go test ./... -p 2 -timeout 2700s`
PASS; `wails build` PASS. On the pushed commit, two frontend and Linux CI
suites and one Windows suite passed; the second Windows CI suite was pending
when this record was written. **Installed VM startup, Chinese tray behavior,
and user-data retention for this replacement candidate remain unverified.**

## Version identities

- `v1.1.1` is the proposed **desktop product/release tag**. Existing manually
  distributed development builds display `1.1`; the next development build
  displays `1.1.1`. The patch increment is deliberate: old `1.1` binaries
  compare `1.1.1` as newer, whereas they consider `1.1.0` equal.
  `wails.json`, the frontend package, and Windows file metadata carry the
  matching numeric forms. The release workflow checks these before building.
- `DesktopReleaseTag` is empty in a source-built/development desktop app and
  is stamped as `v1.1.1` in the tagged app binary. A development copy at
  `1.1.1` may be offered that official release once; the official build does
  not offer itself. This marker is not a peer protocol version.
- `internal/version.Version` is the inherited **core/peer build identity**
  (`2.3.1` today). LAN/WAN peer advertisements and CLI output use it. The
  desktop no longer offers peer binaries as product updates based on this
  identity; the legacy peer-install binding rejects requests. A GameSave Go release tag must not be injected into this
  value. Tagged builds still stamp `BuildTime` so same-core-version peers can
  distinguish build age. Any future change to peer versioning requires a
  compatibility migration and a mixed-version two-device test.
- Linux, Steam Deck, CLI, relay, and internal `opensave` identifiers remain
  compatibility surfaces. A Windows installer may use the GameSave Go name;
  that does not authorize a data-path or protocol rename.

## Publication gate

Pushing a matching `v*` tag can run the build jobs, but the GitHub Release job
is skipped unless the repository variable `GAMESAVE_GO_PUBLIC_RELEASE_READY`
is explicitly set to `true`. The owner has chosen an unsigned Windows release
policy for this personal open-source project: no commercial certificate is
required, and Windows may display SmartScreen warnings or block installation
under device policy. The workflow must attest final build bytes, verify the
downloaded artifacts against the repository/workflow/ref/commit, and publish
`SHA256SUMS` for every asset. This variable is **not** a substitute for the
release review below. Do not set it or create a release tag until the remaining
blockers have been signed off. The preflight accepts only an exact stable tag,
not a beta suffix. The tagged path has not yet run.

The same workflow also defines a manual Windows **candidate-only** run with a
required `candidate_version` input, currently `v1.1.1`. It checks the input
against package metadata, requires a branch ref, builds the NSIS installer and
portable executable, attests their final bytes, downloads them in a separate
job and verifies the exact source identity. It does not stamp the branch name
as a desktop release identity, run Linux/Flatpak release jobs or publish a
GitHub Release. The Actions artifact is a temporary test candidate, not an
official download. Record its workflow run URL and verification outcome.

Before enabling publication, verify on an isolated clean Windows VM:

1. Build a candidate installer and portable executable; inspect ProductName,
   product/file versions, icon, unsigned-publisher/SmartScreen behavior and
   SHA-256 checksums. Confirm installer payload is present and launch succeeds.
2. Install as a normal user. Record data and credential locations, then
   install the candidate over a previous build. Confirm games, snapshots,
   protected cloud credentials, and paired devices survive.
3. Exercise update detection and a release-asset download. Test both portable
   replacement and protected-directory installer/UAC path. The desktop code
   now restricts the selected asset and checksum file to this repository and
   release, then checks the downloaded size and SHA-256 before use. Synthetic
   tests cover failures; real GitHub redirects and installed-binary behavior
   remain unverified. A checksum published alongside an asset is not
   independent provenance; verify its GitHub attestation as well.
   An older `1.1` development binary should see `v1.1.1` as newer, while a
   source-built `1.1.1` copy should see the same-version release through the
   absent release marker. Verify both paths on installed builds; unit tests
   do not substitute for that upgrade test.
4. Exercise rollback to the previous build **without** rolling back or
   deleting user data. If a schema migration makes rollback unsafe, document
   the exact compatibility boundary and provide a tested recovery procedure.
5. Run a mixed-version device-sync test. A peer running inherited core
   `2.3.1` must not offer a desktop binary update or interrupt save sync.
   Confirm desktop update notices come only from this fork's GitHub releases.
   Inspect installer and uninstaller in both English and 简体中文, and switch the
   app language on a running VM to verify all tray labels and tooltip.
6. Review `LICENSE`, `NOTICE.md`, dependency notices, embedded links, and
   release notes. Scan the candidate artifacts and repository for credentials,
   tokens, private accounts, save data, and development-only paths.
7. Complete the cloud-vault, conflict, interrupted transfer, and restore
   safety gates in `TASKS.md`. The successful owner-reported small-save
   Jianguoyun cross-device test is valuable but does not cover these cases.

## Channels and recovery

Stable is the only planned public channel for this workflow. Beta selection
exists in the app, but a GameSave Go beta release is not enabled until desktop
pre-release versioning, metadata, and upgrade ordering are tested. Do not
present candidate workflow artifacts as supported downloads. Keep a previous
installer and checksum available for recovery; never roll back live
saves or SQLite data merely to roll back a binary. A failed upgrade should leave the
old executable and user data recoverable.

## Evidence record

For each candidate, record the tag/commit, Windows VM image, installer hash,
attestation identity and unsigned-publisher warning, installation/upgrade/rollback outcomes, paired-device
versions, test results, and any `PASS`, `FAIL`, or `BLOCKED` item. Do not fill
unexecuted checks with inferred passes.

## Local preflight on 2026-09-29

- **PASS (scoped):** v2 archive imports skip untracked games before staging or
  writing a save, in both modes. The UI and CLI explain that the game must be
  tracked at a locally chosen path first. Existing tracked-game restores still
  use their verified safety-snapshot path. This is not a general atomic restore
  guarantee.
- **PASS (limited source scan):** 463 tracked files contained no tracked
  `.exe`, `.dll`, `.pfx`, `.p12`, `.pem`, `.db`, `.sqlite` or `.sscb` file, and
  fixed searches for common private-key / token prefixes found no match.
  This is not a full history scan or proof that no secret exists elsewhere.
- **PASS / FAIL (dependency audit):** `npm audit --omit=dev` reported zero
  production-classified findings, but full `npm audit` reported 12 development
  and build-tool findings (8 moderate, 3 high, 1 critical). These need triage
  and remediation before a public release; production classification does not
  remove build-tool risk. The Go vulnerability check was **BLOCKED** because
  the official Go module proxy was unreachable while fetching `govulncheck`.
- **PASS (release-finalization branch, 2026-09-29):** refreshed the frontend
  build dependencies to Svelte 5, Vite 8 and Vitest 5. `npm ci`, all 117
  frontend tests, the production build, and local Wails build passed with a
  Node 24 runtime. Full `npm audit` now reports zero findings. The host's old
  Node 20.10 cannot run the new toolchain; CI uses Node 22. A retry of the Go
  vulnerability scan remains **BLOCKED** by a timeout reaching the official
  Go module proxy. The complete `go test ./...` also passed, including e2e.
- **PASS (local packaging only):** a local portable Wails candidate builds at
  `cmd/opensave-app/build/bin/GameSaveGo-preflight.exe` (23,127,552 bytes;
  SHA-256 `5BA60C3C8E6BE8ABD526426487E70456F10FB08D68FD43C08B34DC91600CA99B`).
  NSIS 3.12 was downloaded through WinGet with a verified hash and extracted
  to a task-specific temporary directory, without installing it on the host.
  `wails build -nsis` produced `GameSave Go-amd64-installer.exe` (1.1.1,
  12,473,913 bytes; SHA-256
  `310FE40E80B638A86FF99410305A136349FF06E69C0560D6906EC6B8838B251F`).
  A 1.1.0 comparison installer was built from commit `21fc0c0` in an isolated
  worktree and copied alongside it as
  `GameSave Go-1.1.0-baseline-installer.exe` (12,424,105 bytes; SHA-256
  `957728DA16A70BCEB3D593D8017944321DD39E3E3BDD40F50C87A76EA06BB8DF`).
  Archive inspection found the app executable and WebView2 bootstrapper in
  the 1.1.1 installer. Both installers are unsigned and are test fixtures,
  not release artifacts. The portable executable's Windows version fields
  remain empty, while the 1.1.1 installer advertises ProductVersion/FileVersion
  `1.1.1`.
- **SKIPPED in the earlier pass (truly clean install):** the owner saved the previous VM state,
  switched to a snapshot labelled pre-install, and copied both local test
  installers into the VM. Subsequent logs proved that this Windows profile
  already contained GameSave Go data and earlier cloud/P2P activity. The
  reported upgrade/rollback checks below are useful, but a fresh-profile or
  pristine-image installation was untested at that point; see the later
  owner-reported new-VM check below.
- **PASS (static installer review only):** Wails v2.12.0's generated NSIS
  template writes the app executable to the installation directory. Its
  uninstaller recursively removes that directory and the separate WebView2
  cache under `%APPDATA%`, while the app resolves its SQLite database and
  backup directory under the user's `%USERPROFILE%\.opensave`. No store or
  schema implementation changed between the local 1.1.0 baseline commit and
  this 1.1.1 candidate. These source-level observations are not evidence that
  an actual install, upgrade, or rollback preserves user data.
- **PASS after restart / initial failure (baseline VM launch):** the owner
  installed and opened the locally built 1.1.0 comparison package; the UI
  displayed `v1.1` but initially reported that the local service did not
  start. After a VM reboot it opened the home screen without that error.
  The owner subsequently reported that a separate test game appeared with a
  local snapshot in the baseline app, created a separate pre-upgrade VMware
  snapshot, and disabled the VM network. After an in-place install of the
  locally built 1.1.1 candidate, the owner reported that the app displayed
  `v1.1.1`, the test game and prior local snapshot remained, and the test
  file was unchanged. This is an owner-observed upgrade PASS, not an
  independent file-hash or installed-binary-hash measurement. The owner then
  changed only the synthetic test file, observed a new snapshot on 1.1.1,
  confirmed the Jianguoyun application password still displayed as
  configured (without disclosing it), and saved another pre-rollback VM
  snapshot. This is a UI-state credential-retention check, not a live account
  authentication check because the VM network was disabled. The owner then
  installed the 1.1.0 baseline over 1.1.1 without reverting the VM state and
  reported `v1.1`, both the older and 1.1.1-created local snapshots, the
  synthetic file's B content, and the protected password still showing
  configured. This is an owner-observed binary-rollback PASS; independent
  hashes remained open. The owner then reinstalled the 1.1.1 candidate and
  reported that the displayed version was `1.1.1`, the snapshots and test
  file remained, and the Jianguoyun password still displayed as configured.
  The final re-upgrade is an owner-observed PASS for those items, still
  without a live cloud authentication check or independent file hashes.
  VMware snapshots were for disaster recovery only, not binary rollback
  evidence.
  The supplied startup log shows three previously tracked games, historical
  cloud/P2P activity, and an existing `.opensave` data directory; the selected
  VM state is therefore **not clean user data**, regardless of its snapshot
  label. On the 1.1.0 launch, it records `1.1.1 → 1.1` but not the later
  successful local-service connection line. Older cloud/P2P errors in the log
  do not establish the cause of the initial startup failure. The clean
  relaunch after reboot does not prove what caused the first error, and this
  VM state must not be counted as clean-user-data installation evidence.
- **OPEN (credential scope):** Jianguoyun's application password has protected
  storage and no password readback, but generic WebDAV password and custom
  client-secret fields still have SQLite/settings-response paths. Review those
  before a public installer.

Executed checks on this working tree:

| Check | Result |
| --- | --- |
| Focused v2 backup import API and fresh-install CLI tests | PASS — untracked games skip; tracked games recover into explicitly chosen local folders |
| `go test ./... -timeout=20m` | PASS — the first full run exposed two obsolete E2E expectations of untracked overwrite; after updating the tests and CLI guidance, the second full run passed |
| Frontend `npm test` | PASS — 18 files, 113 tests |
| Frontend `npm run build` | PASS |
| `wails build -o GameSaveGo-preflight.exe` and `wails build -nsis` | PASS — unsigned portable and 1.1.1 NSIS installer built locally |
| Historical 1.1.0 comparison installer | PASS — built from isolated commit `21fc0c0`; not an official prior release |
| Clean VM install/upgrade/rollback and save/credential retention | PARTIAL — earlier owner-reported 1.1.0 → 1.1.1 → 1.1.0 → 1.1.1 retained snapshots, test file and the offline protected-password configured indicator, but that VM had existing user data. A later new-VM Chinese install/tray/uninstall check passed per the owner; clean-VM upgrade/rollback, independent guest hashes and live credential usability remain unverified |
| `npm audit --omit=dev` / full `npm audit` | PASS — zero production-classified findings / FAIL — 12 development-build findings |
| Go dependency vulnerability scan | BLOCKED — official module proxy connection failed before the scanner could run |

Decision: keep these builds as **owner-only, unsigned test candidates**.

### 2026-09-29 desktop update and localization follow-up

The desktop's GitHub update checker was already scoped to
`lyx5710317/gamesave-go`; the apparent upstream `2.3.1` update came from the
Devices screen comparing inherited peer/core build metadata. That screen no
longer offers peer binary updates, and its legacy native install method now
rejects calls. Save synchronization and peer metadata remain intact. The
tray receives the UI's saved language at startup and subsequent changes. A
maintained NSIS template now offers English/简体中文 installation language and
localizes the Wails prerequisite/WebView messages. Installer language must
be chosen before app data exists, so it cannot inherit the desktop setting.

Checks on the follow-up working tree: focused Go/frontend tests PASS;
`go test ./...` PASS (E2E 599.502 s); frontend `npm test` PASS (18 files,
114 tests); `npm run build` PASS; `wails build` PASS; `wails build -nsis`
PASS. The locally generated 1.1.1 installer reports GameSave Go and version
1.1.1, but remains **unsigned**. The owner subsequently reported PASS on a
**new Windows VM** for Chinese installation, Chinese tray labels and retention
of a synthetic save after uninstall. This is owner-observed only, without
independent guest hashes or credential checks. English installer pages,
upgrade/rollback on that clean VM and live cloud-credential use remain open.
The owner reports no Windows code-signing certificate. No GameSave Go GitHub
release was published.

The source branch was pushed to GitHub in three phase-focused commits.
The branch-ref, unsigned Windows candidate workflow
([run 36534491985](https://github.com/lyx5710317/gamesave-go/actions/runs/36534491985))
passed its preflight and Windows jobs; Linux, Flatpak and public release jobs
were skipped as designed. The workflow artifact exists, but its download and
guest binary hash have not been independently inspected.

After that run, `npm audit fix` without `--force` updated only the Nano ID
and PostCSS lockfile entries. Frontend tests (114) and build passed, and the
full audit fell from 12 to 10 findings (8 moderate, 1 high, 1 critical).
The remaining fixes require Svelte/Vite/Vitest major upgrades; no forced
upgrade was attempted. `npm audit --omit=dev` still found zero. The
`govulncheck` tool could not be fetched from either `proxy.golang.org` or
direct `golang.org` on this host, so the Go vulnerability scan remains
BLOCKED, not clean.

The updated branch then passed a second unsigned Windows candidate workflow
([run 36537434811](https://github.com/lyx5710317/gamesave-go/actions/runs/36537434811))
at commit `aa90d3d`; the `windows` artifact is available in that run's
Artifacts section. The publication job was skipped. A local rebuild after
the lockfile refresh passed, but its installer is unsigned. Do not represent
either historical candidate as a formal release; both predate the new
attestation gate.

The unsigned, attested Windows candidate at commit `77ef534` passed the
[branch-only GitHub run 36543874720](https://github.com/lyx5710317/gamesave-go/actions/runs/36543874720)
on 2026-09-29. Its Windows job produced and attested the final installer,
portable app, CLI and relay executables. A separate Ubuntu job downloaded the
artifact and verified all four bytes against the expected repository, workflow,
branch ref and commit. Preflight passed; Linux, Flatpak, relay Docker and public
release jobs were skipped as designed. This demonstrates Windows candidate
provenance only, not a public release, a Windows publisher signature, or the
still-unrun Linux/Flatpak/SHA256SUMS publication path. A local attempt to
download the candidate artifact for independent Windows inspection stalled on
this host; no local executable hash or guest test is claimed for this run.

A second branch candidate at commit `127e1c5` passed
[run 36546639311](https://github.com/lyx5710317/gamesave-go/actions/runs/36546639311)
on 2026-09-29. The separate verification job passed again and logged these
SHA-256 values for its downloaded Windows files (candidate-only, not Release
assets):

| File | SHA-256 |
| --- | --- |
| `GameSaveGo.Setup.exe` | `e8ce3b55acda3d6b643a0745cfdbf69d458a7270f411ef23d40b8bbe903d5aed` |
| `GameSaveGo.exe` | `cffad438a5c266c8aeea71b8a1d3a64b467ab6c837ac0b0983332b2188aa3daa` |
| `opensave-cli.exe` | `5362e786de5f40996d6fdeea276d846c512c811e3d758811cf5fe5c367a08270` |
| `opensave-relay.exe` | `b3475f708260b2e5ac06ce2fa7e97234959f6824e9a6e1cc36d6aa0a204cd79c` |

Local final checks for this policy change: focused release-workflow Go test
PASS; `go test ./...` PASS; frontend `npm test` PASS (114 tests); frontend
`npm run build` PASS; `wails build` PASS. The public tag/release workflow and
release manifest attestation remain **SKIPPED** because no tag was created and
the explicit publication gate is not enabled. This candidate has not been
installed or independently downloaded to the local Windows VM.

Do not publish a formal installer or public test download yet. The remaining
dependency/security findings, tagged-path attestation rehearsal and clean-VM matrix must be
verified on the same candidate first:

1. From a known clean VM snapshot, install a prior reviewed installer as a
   normal user. Create only a synthetic test save; record its SHA-256 and a
   local safety snapshot. If testing cloud credentials, enter a disposable
   credential yourself in the VM and record only its configured/not-configured
   state, never its value.
2. Upgrade **in place** using the new candidate installer. Without reverting
   the VM snapshot or deleting app data, confirm the game, save hash, snapshot
   history and protected credential remain usable. Record installer hashes
   and the Windows account used.
3. Roll back **only the program binary/installer**, not the user database,
   Credential Manager or save folders. Recheck the same items, then upgrade
   again and recheck. If an older build cannot read the newer data format,
   stop and document the minimum rollback version; do not repair by copying
   plaintext credentials into SQLite.
4. Report each step as PASS/FAIL/BLOCKED with the actual binary hashes and
   non-secret observations. Restoring the whole VM snapshot is not evidence
   of a safe binary rollback because it restores old user data too.
