# AGENTS.md — Hub Repo

Hub monorepo: API server, web console, DB migrator and shared Go packages.

## Read first

Codex (GPT-6 Astra) and Claude Code (Opus 5.5) both load this file. Edit
`AGENTS.md` only; `CLAUDE.md` is a relative symlink to it.

This file covers routine work in this repo. The workspace guide,
`../AGENTS.md` (from a worktree:
`$HOME/Development/LabTether/AGENTS.md`), has the full rules, lab
machines and release stages. Read it before release, signing, QA, lab or
cross-repo work. If the two disagree, the workspace guide wins; fix the stale
line here.

## How to work

- Carry the task to a checked result. Make routine, reversible choices
  yourself. Stop to ask only for a decision that is the owner's or a boundary
  below, and keep doing independent work while you wait.
- Check the branch and dirty files before editing; leave unrelated work alone.
  Reuse this checkout. Worktrees go under
  `$HOME/.codex/worktrees/LabTether/`; throwaway files go in the
  task's `work/` folder or `mktemp -d`, never directly in `$HOME` or
  `$HOME/Development`. Clean up only what you created.
- Run one broad build or suite at a time and reuse existing caches.
- Reply in short, plain words: what changed, whether it worked, what is next.
- User-facing text follows "No AI slop" below.

## No AI slop (owner rule)

Applies to every word a person reads: UI, CLI output, log and error messages,
docs, README, release notes, store listings, mockups and demo data. These are
the workspace "No AI Slop" rules in `../AGENTS.md`; keep the two in sync.

- Keep normal homelab words: device, node, host, hub, agent, VM, LXC,
  container, stack, pool, dataset, snapshot, backup, restore, SSH, console,
  VNC, RDP, uptime, latency, logs, alerts, webhooks, cron. Keep page names.
- State facts LabTether knows. No guesses ("probably", "likely", "should be")
  and no advice paragraphs. Put the fix in a button or a command.
- Do not explain or restate the screen: no subtitle that repeats the list's
  counts, no "Here you can…", no "You get an email for each".
- Labels are a verb or a noun ("Start VM", "Snooze"), not a sentence. Status
  is a word plus a time: "Offline since 21:12", "Recovered at 14:05".
- Cut hype: seamless, effortless, powerful, robust, blazing fast, magic,
  supercharge, unleash, elevate, next-level, game-changer, best-in-class,
  single pane of glass, mission control, peace of mind, "at a glance".
- Cut filler: simply, just, "That's it.", "No more X", "Say goodbye to X",
  "everything you need", "takes care of the rest".
- Cut structure tells: "X, Y, Z — tagline", "Not just X. It's Y.", questions
  as headings, em dashes used as commas, stacked adjectives, lists of three
  by habit.
- Cut internal words from user text: canonical, operator, lane, bucket,
  projection, entitlements, reconciliation, mutation, orchestration,
  provenance, dogfood, "source of truth", handoff, surfaces, preflight. Show
  no job or request IDs, raw event codes, bare status codes or API routes.
- No made-up numbers, fake testimonials or lorem ipsum. Demo data uses
  made-up host names, never the owner's real machines.
- Visuals: no icon in a coloured box as decoration, no rainbow charts, no
  decorative blobs, gradient washes or emoji icons. Text meets WCAG AA 4.5:1.

Read user-facing text once against this list before you ship it.

## Boundaries

- Run VMs, Docker stacks, simulator QA, installed-agent tests and long suites
  on the lab test machines named in the workspace guide, never on the
  development laptop. One test file, lint or one compile is fine locally.
- Machines the workspace guide marks as no-touch are out of scope: do not
  query, operate on or cite them.
- Signing material stays in its local-only folder outside every repository.
  Never list, copy, print, stage or upload any of it. Signing and notarization
  need the owner's go-ahead for each release.
- Removing repos, branches, worktrees, user files or dirty work needs an exact
  preview and the owner's approval.
- Prefer a scoped disposable token or an existing session to changing auth.
  Before any temporary auth change, set restore traps and confirm the original
  state afterwards. Never rotate the owner password when a safer route exists.
- In `zsh`, `status` is reserved; name exit codes `rc` or `exit_code`.
- Keep personal details out of files, test data and commit messages: real
  names, home folders, lab host names, home IP addresses, personal emails and
  tailnet names. Use neutral examples such as `/Users/demo`, `nas01` and
  `user@example.com`. `python3 scripts/ci/check-private-info.py` and CI block
  them; the workspace git hooks also block the exact private values.

## Checks

- Hard limit: 500 code lines per handwritten file (source, tests, scripts,
  executable CI/build/config), counted by pinned `cloc 2.10`. Run
  `python3 scripts/ci/check-line-limit.py` here. Split by responsibility; do
  not minify, number chunks or move code into data. Existing violations stay
  open until the checker passes.
- Add or keep a test only when it catches a real fault or guards a user
  promise, and name that risk. Skip getter-only, duplicate and
  implementation-mirror tests. Docs-only edits need path, link, diff and
  line-limit checks, not builds.
- Before starting or rerunning remote CI, check the remaining minutes and
  spend cap and name what the run proves. Do not rerun for another green.
- Report what actually passed. A build does not prove runtime behavior, a mock
  does not prove a live result, and a backup is not a restore.

## This repo

- API and runtime wiring: `cmd/labtether/`. Reusable behavior: `internal/`,
  including `internal/hubapi/`. Migrations: `services/migrator/`.
- Console: `web/console/`. Keep user-facing text in the existing i18n system
  and keep the authentication, API and realtime event contracts intact.
- Go: test the changed package, for example `go test ./internal/hubapi/agents`.
- Console: from `web/console/`, run the affected Vitest file with
  `npm test -- <test-file>`, and `npm run lint:types` for TypeScript changes.
- Broader required checks live in `Makefile` and the matching CI workflow.
  Live integration, backup, restore and remote-access scripts need a known,
  allowed lab target.
- Architecture: `docs/internal/`. Planning: `notes/TODO.md`. Dated QA and
  release notes go stale; recheck them against the current branch and runtime
  before claiming readiness.
- Keep Hub's `github.com/labtether/protocol` pin equal to the Go agent's. Hub
  releases only after the workspace prerequisite stage and its exact-version
  public-asset checks pass.

## Agent distribution

Agent binaries are not stored in this repo. CI downloads them from GitHub
Releases and bakes them into the image.

- No `build-agent-*` Makefile targets; agents build in their own repos.
- `agent-manifest.json` tracks versions and checksums.
- `AgentCache` looks in `/data/agents/` (runtime cache), then
  `/opt/labtether/agents/` (baked into the image).
- Endpoints: `GET /api/v1/agent/binary`, `GET /api/v1/agent/releases/latest`,
  `GET /api/v1/agent/manifest`, and admin-only `POST /api/v1/agent/cache/refresh`.
- Settings: `LABTETHER_AGENT_DIR` (default `/opt/labtether/agents`) and
  `LABTETHER_AGENT_CACHE_DIR` (default `/data/agents`). There is no runtime
  GitHub refresh; `LABTETHER_AGENT_MANIFEST_REFRESH` was removed in 1.7.0.

## Sibling repos

All live under `../`: `website/`, `labtether-agent/` (Go agent), `protocol/`,
`mac-agent/`, `win-agent/`, `ios/`, `labtether-cli/`,
`labtether-homeassistant/` and `labtether-linux/`. The workspace guide has the
full map.
