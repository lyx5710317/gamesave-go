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
Actions candidate provenance, tagged build, published download hashes and
installer integrity are pending.

Official downloads must come from the repository's Actions-built Windows release.
Publish only GameSaveGo.Setup.exe, GameSaveGo.exe and SHA256SUMS, with exact
repository/workflow/ref/commit provenance verification. Binaries are unsigned;
the README and release notes disclose SmartScreen and device-policy restrictions.
The existing publication gate will be enabled for the intended tag run only and
returned to false after publication. No InkPage website publication is requested.
