# GameSave Go v1.2.1 release record

Date: 2026-10-05. Windows-first patch release requested by the owner, with an
InkPage draft article and verified download links. Desktop version is 1.2.1;
inherited core/peer versions, storage identifiers and provider foundations remain
compatible. The source audit is recorded in TASKS.md.

## Changes

- Verify complete safety-snapshot contents before incoming sync and keep-remote
  replacement, using the existing restore preflight.
- Validate local ZIP paths before clearing current saves, sharing the cloud
  validator for traversal, aliases, duplicate paths and file/directory collisions.
- Report local file-deletion failures instead of continuing with false success.
- Restrict dashboard REST/WebSocket origins and DNS-rebinding hostnames.
- Escape OAuth page details, require single-use callback state, and scope callback
  listener shutdown to its own authorization attempt.
- Preserve literal special-character SQLite filenames without changing schema.
- Require Go 1.26.6, eliminating seven reachable standard-library advisories found
  by govulncheck on the previous Go 1.26.4 build.

## Verification and limits

- PASS: audit regression tests and Go 1.26.6 full suite, including E2E in 608.355s.
  The final repeat includes the late Windows credential deletion/isolation test.
- PASS: audit frontend 129 tests, frontend production build, Wails Windows build,
  go vet, npm audit (zero findings), and diff whitespace checks.
- PASS: Go 1.26.6 govulncheck reports zero reachable advisories and zero advisories
  in imported packages; 21 required-module-only findings remain disclosed.
- PASS: Linux cross-compilation of API/store/cloud/snapshot/syncengine test
  binaries. This does not execute Linux tests.
- FAIL in an initial audit run: one intermittent protected-header deletion
  assertion. It subsequently passed 20 repeats; an actual-password isolation
  regression passed 100 delete cycles. Root cause is unconfirmed and no production
  credential fix is claimed. Historical Windows one-sided-sync flakiness remains
  tracked separately.
- SKIPPED locally: race detector because this environment has CGO disabled.
- SKIPPED: clean-VM installation/upgrade, installed-device UI, actual OAuth/cloud
  provider matrix and multi-root I/O-failure recovery. Safety verification does
  not lock out concurrent writers after its final scan or add a global cross-root
  transaction.

## Release preparation

Version metadata, package/lockfile roots, Windows resources and update-ordering
tests are updated together. PASS: versioned `go test ./...` (unchanged packages
reuse audited results), 129 frontend tests, frontend build, and Wails Windows
build in 21.379s. Linker-stamped v1.2.1 identity and update-ordering tests PASS.
PASS: [candidate run 37266456455](https://github.com/lyx5710317/gamesave-go/actions/runs/37266456455)
from `658adc8fc11f688a9c21ee5ef9a013e61c73d5b8`. Both its independent workflow
verification job and local `gh attestation verify` checked all four Windows
executables against that exact branch, commit, repository and release workflow.
Candidate installer and portable ProductVersion are 1.2.1; the NSIS payload
extracted without errors. These candidate bytes are not the tagged download.

PASS: [push CI 37266213547](https://github.com/lyx5710317/gamesave-go/actions/runs/37266213547)
and [PR CI 37266463608](https://github.com/lyx5710317/gamesave-go/actions/runs/37266463608):
frontend, Linux race suite and Windows race suite. The push Windows E2E result
was 1135.604s. Hosted builds use Go 1.26.8; local audit scans remain Go 1.26.6.
These CI passes do not establish an installed Linux/Steam Deck UI check or explain
the earlier intermittent Windows failures.

## Published release verification

PASS: [v1.2.1](https://github.com/lyx5710317/gamesave-go/releases/tag/v1.2.1)
was published on 2026-10-05 from tag commit
`23b020423d18b1e5d1dda5f6974494a346d2de1c` in
[release run 37268557484](https://github.com/lyx5710317/gamesave-go/actions/runs/37268557484).
The merged tag tree is identical to the tested candidate tree. The complete tag
workflow passed, including Windows, Linux, relay and Flatpak builds and the
Windows public-release job. No Linux or Flatpak packages are presented as
validated public downloads in this release; build/linked-library checks do not
establish an installed Linux or Steam Deck UI check.

PASS: independently downloaded installer and portable files match SHA256SUMS and
report ProductVersion 1.2.1. All three published assets passed local provenance
verification constrained to the repository, `refs/tags/v1.2.1`, exact tag commit
and release workflow. The installer payload extracted successfully; its Go build
metadata confirms Go 1.26.8 and the official `DesktopReleaseTag=v1.2.1` stamp.
This is package integrity/source verification, not an installed-VM smoke test.

Final SHA-256:

- GameSaveGo.exe: `2ff5025287802a6cc0e6909acd52ca6554cd7984af550e1dc0f7e348a0ed4546`
- GameSaveGo.Setup.exe: `d935897787323149e6ee6a93054a2829497b54482592ba8ab82e4648726c6ead`
- SHA256SUMS: `322a1c877d8eed75a1475fb22787c681de1075766fc0c7fead7012d89b238a21`

PASS: the intended-tag publication gate was returned to false after the public
release job completed. A Chinese update article with the actual three download
URLs was saved in the existing InkPage draft vault with draft=true. It was not
synced to the published article directory or deployed to the website.

Official downloads must come from the repository's Actions-built Windows release.
Publish only GameSaveGo.Setup.exe, GameSaveGo.exe and SHA256SUMS, with exact
repository/workflow/ref/commit provenance verification. Binaries are unsigned;
the README and release notes disclose SmartScreen and device-policy restrictions.
The existing publication gate was enabled for the intended tag run only and
returned to false after publication. No InkPage website publication was requested.
