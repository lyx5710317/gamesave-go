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

PASS: [candidate run 36822648674](https://github.com/lyx5710317/gamesave-go/actions/runs/36822648674)
from `fa6c143068d67201ca566ec919454ad81cba2b97`, ref
`refs/heads/codex/release-v1.2.0`. Both the independent workflow verification job
and local `gh attestation verify` checked all four Windows build artifacts against
the repository, ref, exact commit and release workflow. Downloaded desktop files
report ProductVersion 1.2.0; NSIS archive integrity passed.

Candidate-only SHA-256 (these are not hashes of the final tagged release):

- GameSaveGo.exe: `99926e6173ea5d97cbce5f78a92304bc1e0623592e89653df5ab72fe5aef76b9`
- GameSaveGo.Setup.exe: `0912abfaeb33cf244be2a50c6cf584ff87bbf94ee9273612d3319d0f8778bc0c`

Running the downloaded candidate was SKIPPED after automatic approval review
rejected the command to pause the existing app and launch it (`blocked by policy`).
No bypass or retry through another execution mechanism was attempted. Local
package-payload smoke testing above remains distinct from downloaded-byte checks.

PASS: [PR CI run 36822649461](https://github.com/lyx5710317/gamesave-go/actions/runs/36822649461) on the same candidate commit: frontend, Linux race suite and Windows race suite. The duplicate push run 36822612296 failed TestMultiFileSync_OneSidedEditsNeverConflict while its other packages passed; a rerun was requested. Preserve this intermittent failure as a follow-up; do not classify the failing run as PASS.

PASS: browser interaction smoke checks using synthetic local API fixtures: Chinese title opened the encoded Black Myth: Wukong FLiNG URL; unknown title displayed manual input, Chinese input kept search disabled, and Elden Ring enabled the encoded English search.

Pending: tagged Windows build, public release and
downloaded-release verification.
Publish only the Actions-built Windows installer, portable app and SHA256SUMS.
Windows binaries remain unsigned; disclose SmartScreen and the existing cloud,
sync and dependency-scan limits. Linux/Steam Deck/CLI/P2P foundations remain intact.
