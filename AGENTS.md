# AGENTS.md — Hub Repo

This is the hub monorepo for LabTether. It contains the hub API server, web console, DB migrator, and all shared Go packages.

## Shared rules

For Astra and Opus 5.5: edit `AGENTS.md`; keep `CLAUDE.md -> AGENTS.md`.
Read `../AGENTS.md` once if available, or from a worktree use
`/Users/michael/Development/LabTether/AGENTS.md`. Read only task-relevant docs.
Finish scoped work with focused checks; make routine reversible choices yourself.
Use current manifests, preserve unrelated dirty work, and reply in short plain words.

- Reuse this repo; do not clone or copy it. Worktrees belong under
  `/Users/michael/.codex/worktrees/LabTether/`; temp files in task `work/` or
  `mktemp -d`. Create no repos, worktrees, caches or temp folders directly in
  `/Users/michael` or `/Users/michael/Development`. Clean only your own temp files.
- One broad build/suite at a time; require 100 GB free for heavy builds. Reuse
  caches. Removing user files, dirty work, repos, branches or worktrees needs
  an exact preview and explicit approval. Preview generated-cache cleanup.
- VM 102 / `UntrustedVM` is excluded and untouched: no enumeration, queries,
  inspection, backup, operations or QA evidence.
- Signing material stays local outside repos: never expose, list, copy, stage or
  upload it. Release signing/notarization needs explicit authorization; preserve owner
  signing pauses. Read workspace release rules; publish only verified distributables.
- Prefer scoped disposable credentials or an existing session; never rotate the
  owner password if either is available. Before temporary auth changes, install
  restore/cleanup traps, save the exact state without logging secrets, then
  verify restoration of its hash/timestamp and session baseline.
- In `zsh`, use `rc` or `exit_code`, never reserved `status`.

## File size and checks

- Hard limit: 500 code lines per handwritten source/test/script or executable
  CI/build/config file, using pinned `cloc 2.10` (excludes blanks/comment-only
  lines). Only genuine generated/vendor code is exempt. No legacy exceptions,
  minifying, numbered chunks or moving code into data; split by responsibility.
- Run `python3 scripts/ci/check-line-limit.py` here. Existing violations remain
  open until it passes. Use focused tests and required CI; broaden for shared
  behavior or unresolved failures. Builds do not prove live behavior; backup or
  verification does not prove restore. Report what actually passed.

## Work and checks

- API/runtime wiring: `cmd/labtether/`; reusable behavior: `internal/`, including
  `internal/hubapi/`; migrations: `services/migrator/`.
- Console: `web/console/`. Keep operator-facing text in the existing i18n
  system and preserve authentication, API, and realtime event contracts.
- Run Go tests for the changed package, for example
  `go test ./internal/hubapi/agents`. For console work, run the affected Vitest
  test from `web/console/` with `npm test -- <test-file>` and use
  `npm run lint:types` for TypeScript changes.
- Use `Makefile` and the applicable CI workflow for broader required checks.
  Live integration, backup, restore, and remote-access scripts need a known
  allowed target. A successful backup or verification is not a restore proof.
- Read `docs/internal/` for relevant architecture, and `notes/TODO.md` for
  planning. Recheck dated QA/release notes against the current branch and
  runtime before claiming readiness.
- Keep Hub and Go-agent protocol pins aligned. Release Hub only after the
  workspace prerequisite stage and exact-version public-asset checks pass.

## Workspace Navigation

This repo (`hub/`) is part of the LabTether multi-repo workspace at `../`. See `../AGENTS.md` for the full workspace map.

| Sibling | Path | Description |
|---------|------|-------------|
| Public website | `../website/` | Marketing site, docs, pricing (labtether/labtether-website) |
| Go agent | `../labtether-agent/` | Cross-platform endpoint agent (labtether/labtether-agent) |
| Protocol | `../protocol/` | Shared wire protocol types (labtether/protocol) |
| macOS agent | `../mac-agent/` | Native Swift agent (labtether/labtether-mac) |
| Windows agent | `../win-agent/` | Native .NET agent (labtether/labtether-win) |
| iOS app | `../ios/` | iOS mobile app (labtether/labtether-ios) |
| CLI | `../labtether-cli/` | Cobra-based CLI tool |
| Home Assistant | `../labtether-homeassistant/` | HA custom component and add-on |

## Agent Distribution

Agent binaries are NOT stored in this repo. They are downloaded from GitHub Releases at build time and cached at runtime.

### Key Points
- No `build-agent-*` Makefile targets — agents are built in their own repos
- Version tracking uses `agent-manifest.json` (not `LABTETHER_AGENT_RELEASE_VERSION` env var)
- `AgentCache` resolves binaries: `/data/agents/` (runtime cache) → `/opt/labtether/agents/` (baked-in fallback)

### API Endpoints
- `GET /api/v1/agent/binary` — serves cached Go agent binary
- `GET /api/v1/agent/releases/latest` — returns version/checksum from manifest
- `GET /api/v1/agent/manifest` — returns full agent manifest (all agents)
- `POST /api/v1/agent/cache/refresh` — admin-only, reloads manifest from disk

### Environment Variables
- `LABTETHER_AGENT_DIR` — baked-in binary directory (default: `/opt/labtether/agents`)
- `LABTETHER_AGENT_CACHE_DIR` — runtime cache directory (default: `/data/agents`)
- `LABTETHER_AGENT_MANIFEST_REFRESH` — enable runtime refresh from GitHub
