# Repository Development Rules

These rules apply to every human or automated contributor in this repository.

## Before changing code

1. Read the relevant existing implementation, adjacent tests, `README.md`, `LICENSE`, `SPEC_V2.md`, and `TASKS.md`.
2. Check the current branch and working tree. Never develop features directly on `main`.
3. Establish the existing behavior with focused tests before changing it.
4. Prefer modifying existing code, then extending it. Do not create a parallel implementation of an existing capability.

## Protected foundations

Do not rewrite the snapshot engine, Ludusavi resolver, Steam scanner, store, delta engine, watcher, or P2P stack without a documented architectural necessity. Do not delete P2P, Linux, Steam Deck, CLI, or an existing cloud provider because a current MVP screen does not expose it.

Keep internal `OpenSave`, `.opensave`, database, log, and protocol identifiers until a separate compatibility migration is approved. Avoid broad renames that make upstream merges harder.

## Data and schema safety

- Every SQLite schema change requires a forward migration and tests against existing data.
- Restore and incoming sync must create and verify a safety snapshot before replacing current data.
- Never resolve an uncertain local/remote conflict by timestamp alone or silently overwrite either side.
- Cloud providers must remain modular and must not bypass snapshot, conflict, or restore safeguards.
- User save bytes must never transit a GameSave Cloud-owned OAuth broker.

## Secrets and privacy

Never commit a `SecretKey`, client secret, OAuth/access/refresh token, user save, personal data, real cloud account identifier, or production credential. Do not log secrets or save contents. Public desktop clients may use only provider-approved public-client credentials; confidential exchanges require the reviewed minimal-broker design.

## Tests and commits

Every behavior change needs focused automated coverage. Run the relevant package tests while iterating, then before handoff run:

```text
go test ./...
cd cmd/opensave-app/frontend && npm test
cd cmd/opensave-app/frontend && npm run build
cd cmd/opensave-app && wails build
```

Record PASS, FAIL, or SKIPPED with the real reason. Never substitute an assumption for a build. Keep commits small and phase-focused; do not mix unrelated formatting, refactors, dependency upgrades, or branding changes into feature commits.

## Product constraints

The project is Windows-first, open-source, local-first, bring-your-own-cloud, and accountless. Preserve upstream cross-platform foundations. Follow `SPEC_V2.md` for product behavior and keep `TASKS.md` current. Baidu is the planned mainland-China primary provider; Quark remains a candidate until official third-party native-client support is verified.
