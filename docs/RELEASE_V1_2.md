# GameSave Go v1.2.0 release record

Date: 2026-10-01. Windows-first release requested by the owner; desktop version
is 1.2.0, while inherited core/peer version and storage identifiers are unchanged.

## Changes

- Game details offer FLiNG trainer search through the system browser.
- Resolve English titles using Steam App IDs, bundled/curated names and exact
  Chinese aliases. Other Chinese titles use a unique exact official Steam Store
  search match. Unknown or ambiguous names require manual English input.
- Equal desktop versions never trigger update detection or installation,
  including builds without an official release marker.

## Local verification

- PASS: focused version metadata and update ordering tests.
- PASS: `go test ./...`, including existing E2E results reused where applicable.
- PASS: frontend suite (129 tests) and production build.
- PASS: Wails Windows build and NSIS packaging; installer metadata is 1.2.0.
- PASS: 7-Zip extracted the NSIS payload without errors; the embedded application
  launched using isolated USERPROFILE/APPDATA/LOCALAPPDATA directories.
- PASS: synthetic Chinese game resolved to Black Myth: Wukong through the actual
  packaged application's local API. No real save bytes or cloud credentials used.
- PASS on repeat checks: original package payload displayed a functioning 1.2.0
  game library; a fresh isolated profile also reached the welcome screen. No
  equal-version update banner was observed.
- Initial isolated launch displayed startup failure even though its local API
  was reachable. Subsequent original-payload and fresh-profile checks passed;
  cause remains unconfirmed. A test-process Windows firewall prompt also appeared;
  no firewall permission or security setting was changed by automation.
- SKIPPED: full installer/upgrade testing in a disposable Windows VM; no suitable
  running VM was available. Archive/payload smoke checks are not that validation.

## Distribution checks

Pending: branch candidate build, exact artifact provenance and SHA-256 verification,
tagged Windows build, public release and downloaded-asset verification.
Publish only the Actions-built Windows installer, portable app and SHA256SUMS.
Windows binaries remain unsigned; disclose SmartScreen and the existing cloud,
sync and dependency-scan limits. Linux/Steam Deck/CLI/P2P foundations remain intact.
