# GameSave Go 1.1 release preparation

Status: preparation only. No public installer, signed release, or validated
clean-machine upgrade is claimed by this document.

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
  (`2.3.1` today). LAN/WAN peer advertisements, peer-update comparisons, and
  CLI output use it. A GameSave Go release tag must not be injected into this
  value. Tagged builds still stamp `BuildTime` so same-core-version peers can
  distinguish build age. Any future change to peer versioning requires a
  compatibility migration and a mixed-version two-device test.
- Linux, Steam Deck, CLI, relay, and internal `opensave` identifiers remain
  compatibility surfaces. A Windows installer may use the GameSave Go name;
  that does not authorize a data-path or protocol rename.

## Publication gate

Pushing a matching `v*` tag can run the build jobs, but the GitHub Release job
is skipped unless the repository variable `GAMESAVE_GO_PUBLIC_RELEASE_READY`
is explicitly set to `true`. Even then it refuses to publish without both
Windows signing secrets. This variable is **not** a substitute for the release
review below. Do not set it or create a release tag until all blockers have
been signed off. The preflight currently accepts only an exact stable tag,
not a beta suffix. The workflow has not yet been exercised on GitHub runners.

Before enabling publication, verify on an isolated clean Windows VM:

1. Build a candidate installer and portable executable; inspect ProductName,
   product/file versions, icon, Authenticode signature and timestamp, and
   SHA-256 checksums. Confirm installer payload is present and launch succeeds.
2. Install as a normal user. Record data and credential locations, then
   install the candidate over a previous build. Confirm games, snapshots,
   protected cloud credentials, and paired devices survive.
3. Exercise update detection and a release-asset download. Test both portable
   replacement and protected-directory installer/UAC path. Verify signatures
   and published checksums before considering automatic installation safe.
   An older `1.1` development binary should see `v1.1.1` as newer, while a
   source-built `1.1.1` copy should see the same-version release through the
   absent release marker. Verify both paths on installed builds; unit tests
   do not substitute for that upgrade test.
4. Exercise rollback to the previous build **without** rolling back or
   deleting user data. If a schema migration makes rollback unsafe, document
   the exact compatibility boundary and provide a tested recovery procedure.
5. Run a mixed-version peer-update test. A 1.1 product label must not make a
   peer running core `2.3.1` appear older than an unrelated `1.1` protocol.
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
signed installer and checksum available for recovery; never roll back live saves or
SQLite data merely to roll back a binary. A failed upgrade should leave the
old executable and user data recoverable.

## Evidence record

For each candidate, record the tag/commit, Windows VM image, installer hash,
signing identity, installation/upgrade/rollback outcomes, paired-device
versions, test results, and any `PASS`, `FAIL`, or `BLOCKED` item. Do not fill
unexecuted checks with inferred passes.
