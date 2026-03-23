# LabTether Operations

For day-to-day operator workflows, start with:
- [SOLO_OPERATIONS.md](SOLO_OPERATIONS.md)
- [REMOTE_ACCESS.md](REMOTE_ACCESS.md)

For go-live and public-release prep, use:
- [wiki/operations/production-deployment-checklist.md](wiki/operations/production-deployment-checklist.md)
- [wiki/operations/release-readiness-checklist.md](wiki/operations/release-readiness-checklist.md)
- [wiki/reference/supported-release-matrix.md](wiki/reference/supported-release-matrix.md)

## API Key Operations

### Creating API Keys

API keys are created via the v2 API. Only owner or admin accounts can create keys.

```bash
curl -sX POST https://hub:8443/api/v2/keys \
  -H "Authorization: Bearer ${LABTETHER_OWNER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ci-automation",
    "scopes": ["assets:read", "assets:exec"],
    "allowed_asset_ids": ["asset-id-1", "asset-id-2"]
  }'
```

The response includes the raw key (`lt_<prefix>_<secret>`) exactly once. Store it immediately — it cannot be retrieved again.

Fields:
- `name`: Human-readable label for the key (required).
- `scopes`: List of capability scopes (required). See Architecture section for scope definitions.
- `allowed_asset_ids`: Optional asset allowlist. Omit to grant access to all assets permitted by scope.
- `expires_at`: Optional RFC3339 timestamp for key expiry.

### Verifying a Key

```bash
curl -s https://hub:8443/api/v2/whoami \
  -H "Authorization: Bearer lt_<prefix>_<secret>"
```

Returns the key metadata (name, scopes, allowed assets, expiry) without exposing the raw secret.

### Monitoring Key Usage

API key usage is recorded as audit events in the log store with `source=api_key_audit`. Key creation and deletion events are always logged. Per-request audit logging can be enabled per-key.

Query recent key audit events:

```bash
curl -s "https://hub:8443/logs/query?source=api_key_audit&window=24h" \
  -H "Authorization: Bearer ${LABTETHER_OWNER_TOKEN}"
```

Audit events include:
- `event`: `key_created`, `key_deleted`, `key_used` (if per-request audit enabled)
- `key_id`: The key's stable identifier (prefix)
- `actor`: The operator who created or deleted the key
- `timestamp`: Event time

### Revoking API Keys

Revoke a key by its prefix:

```bash
curl -sX DELETE https://hub:8443/api/v2/keys/lt_<prefix> \
  -H "Authorization: Bearer ${LABTETHER_OWNER_TOKEN}"
```

Revocation is immediate. In-flight requests authenticated with the revoked key will fail on their next scope/auth check. A `key_deleted` audit event is recorded on revocation.

To list all active keys (metadata only — raw secrets are never returned):

```bash
curl -s https://hub:8443/api/v2/keys \
  -H "Authorization: Bearer ${LABTETHER_OWNER_TOKEN}"
```

### Rate Limiting Behavior

v2 API requests are subject to per-key rate limits:

- **Per-IP limit**: 600 requests/minute from a single source IP per key.
- **Global limit**: 3000 requests/minute across all source IPs for a single key.

When a limit is exceeded, the hub returns `429 Too Many Requests` with a `Retry-After` header indicating when the client may retry.

Rate limit counters reset on a rolling 60-second window. Limits are enforced in-process (no external Redis dependency).

---

## SLOs and Sizing

### MVP Service Objectives

- Alert detection latency: <= 60 seconds for critical rules.
- Command dispatch latency: <= 5 seconds for online assets.
- Metrics ingestion success: >= 99% samples over 24h.
- Connector health recovery: retries begin within 30 seconds of transient failure.

### Retention Defaults (Balanced Profile)

- Logs:
  - hot searchable retention: 7 days.
  - compressed retention: 30 days.
- Metrics:
  - raw resolution retention: 14 days.
  - rollup retention (5m/1h): 90 days.
- Audit logs:
  - minimum retention: 180 days.

All retention values are user-configurable per source/integration.

### Noise Reduction Controls

- Per-source drop filters (exact/regex patterns).
- Field-based suppression (for repetitive low-value events).
- Sampling for high-volume sources.
- Severity floor by source (for example, drop `debug` outside troubleshooting windows).

### Recommended Presets

- `small` (mini PC / NAS combo):
  - logs hot: 3 days
  - metrics raw: 7 days
  - rollups: 30 days
- `balanced` (default):
  - logs hot: 7 days
  - metrics raw: 14 days
  - rollups: 90 days
- `extended` (larger storage):
  - logs hot: 14 days
  - metrics raw: 30 days
  - rollups: 180 days

### Sizing Tiers

- `small`:
  - 2-4 vCPU, 4-8 GB RAM, 50-100 GB storage.
  - up to ~25 assets.
- `medium`:
  - 4-8 vCPU, 8-16 GB RAM, 150-300 GB storage.
  - up to ~100 assets.
- `large`:
  - 8+ vCPU, 16+ GB RAM, 500 GB+ storage.
  - 100+ assets with high-frequency telemetry.

### Update Mode Policy Defaults

- `approval_required` default for critical assets.
- `unattended` allowed for non-critical assets with:
  - maintenance window
  - precheck pass
  - rollback hook availability where possible

### Operational Guidance

- Start with `small` or `balanced` preset, then tune with observed ingest rate.
- Enable noise filters before increasing retention.
- Keep audit retention longer than logs/metrics for incident traceability.
- In the console, use the shared `Alerts -> Rules` template catalog to bootstrap heartbeat, CPU, memory, disk, error-burst, mobile, and service-health coverage before hand-tuning custom rules.

### Status Aggregate Performance Guardrails (2026-03-04)

- Service health summary now uses a coordinator count path (`SummaryByHosts`) instead of materializing full grouped service lists just to compute `servicesUp/servicesTotal`.
- Canonical status payload generation now reuses a watermark-keyed in-memory cache for unchanged canonical state + asset set combinations.
- Aggregate response ETag generation uses deterministic field hashing instead of JSON marshal-based hashing to reduce allocation churn on high-frequency polls.
- Dead-letter list + analytics paths now use a projected dead-letter query path (`QueryDeadLetterEvents`) when supported by the log store, with a single-query `QueryEvents` fallback for compatibility.
- Dead-letter totals now use an exact window count path (`CountDeadLetterEvents`) when supported, so `/queue/dead-letters` and aggregate summaries are not capped by projected event fetch depth.
- Status aggregate dead-letter snapshots and recent log-source summaries now use log-watermark keyed caches to avoid repeating expensive aggregation queries when log state has not changed.
- `QueryEvents` callers that only need message/source/timestamp can now set `ExcludeFields` to skip JSON field decode/clone overhead.
- `QueryEvents` callers that only need `fields["group_id"]` (for example group reliability and group-filtered source summaries) now use `FieldKeys=["group_id"]`, which activates a projected-group fast path (`projected_group_id`) and avoids full field-map materialization.
- Group-filtered log queries can now pass `GroupID` + `GroupAssetIDs` to reduce row fan-out in SQL while preserving asset-based group fallback behavior.
- Projected group filter reads currently reuse the existing functional projection index (`idx_log_events_site_projection_timestamp`) until the underlying index name is cleaned up.
- Telemetry overview paths (`/status/aggregate`, `/metrics/overview`) now prefer batch snapshot reads (`SnapshotMany`) to avoid one SQL snapshot query per asset.
- Status telemetry overview now reuses a short-lived asset-fingerprint cache (~2s TTL) to reduce burst `SnapshotMany` query churn during concurrent polling.
- Log store now includes a `source+level+timestamp` index path (`idx_log_events_source_level_timestamp`) to reduce filtered log query scan cost.
- Log store also includes a dead-letter partial index (`idx_log_events_dead_letter_error_timestamp`) for `source='dead_letter' AND level='error'` time-window reads.
- Log store includes `idx_log_events_timestamp_source` to improve recent-window source aggregation scans.
- `/worker/stats` now exposes a lightweight `performance` section with top SQL statements when `pg_stat_statements` is available.
- `/worker/stats?query_limit=all` can be used to capture full query snapshots (up to an internal cap of `5000`) for apples-to-apples delta comparisons.
- when source diagnostics are enabled (`DEV_MODE` or `LABTETHER_DEBUG_LOG_SOURCES`), `/worker/stats` also includes `performance.source_queries_top` aggregated by `scope+caller+mode`.
- Console route mitigation:
  - Logs query requests are debounced (`250ms`) to prevent per-keystroke backend fan-out.
  - Logs route requests now pass `include_fields=0` (when no group filter is active) to trim backend decode work for list rendering.
  - Nodes search uses deferred query evaluation so typing remains responsive during large-card recomputation.
  - Logs and Devices routes emit throttled frontend perf telemetry events (`source=frontend_perf`) for request/compute/render latency tracking over time.
  - iOS app emits authenticated mobile observability events to `POST /telemetry/mobile/client` (`source=mobile_client_telemetry`) covering API latency/error outcomes, realtime reconnect state, and lifecycle health markers.
  - iOS mobile telemetry now flushes in bounded batches with auth/config backoff so observability does not create a POST-per-request amplification loop while the shell is idle or before login completes.
  - iOS operators can disable mobile telemetry in-app via **Settings -> Behavior -> Share Mobile Telemetry**.
  - iOS mobile query builders now escape reserved query delimiters (`&`, `=`, `?`, `+`) for file/log/topology paths to avoid malformed backend requests.
  - iOS terminal/desktop embedded web views clear pending reconnect timers on manual disconnect/unload to avoid ghost reconnect loops.
  - iOS terminal/desktop stream URLs are constructed with URL components (instead of string concatenation) to preserve query correctness and IPv6 compatibility.
  - Home, Devices, Services, and More tab realtime refresh handlers are now visibility-aware on iOS so hidden tabs do not keep doing heartbeat-driven refresh work in the foreground.
  - Shared iOS list/screen backdrops now use a static ambient gradient instead of a forever-drifting full-screen animation to reduce baseline GPU/CPU heat.
- Recommended validation loop after performance-sensitive changes:
  - review [PERFORMANCE_CHECKLIST.md](PERFORMANCE_CHECKLIST.md) first.
  - `go vet ./...`
  - `go test ./...`
  - `cd web/console && npm run -s tsc -- --noEmit`
  - `make perf-gate`
  - run reduced perf-contract threshold checks on apples artifacts:
    - `scripts/perf/backend-hotspot-thresholds.sh --summary <run-dir>/summary.json`
    - threshold runs expect `query_stats_enabled=true`; if false, enable `pg_stat_statements` in the target runtime first.
  - capture targeted query plans for planner/index validation:
    - `scripts/perf/backend-hotspot-explain.sh --scenario projected-group --output <run-dir>/explain-projected-group.json`
  - authenticated `pprof` capture on `/debug/pprof/profile` and `/debug/pprof/heap` under representative load.
  - repeatable apples harness run:
    - `scripts/perf/backend-hotspot-apples.sh --label <tag>`
  - for production-mode console checks, mirror static assets into the standalone bundle before launching the built server:
    - `cd web/console && npm run build`
    - `mkdir -p .next/standalone/.next && rm -rf .next/standalone/.next/static .next/standalone/public`
    - `cp -R .next/static .next/standalone/.next/static && cp -R public .next/standalone/public`
    - `HOSTNAME=127.0.0.1 PORT=4173 node .next/standalone/server.js`
  - locale-prefixed console routes are validated against the raw request URL in `web/console/proxy.ts`; use the standalone launch above when verifying `/en/...` redirects so the middleware behavior matches production.

---

## Testing Strategy

### Goals

- Keep core operations safe (policy, command execution, update workflows).
- Catch regressions early with fast local and CI feedback.
- Validate connector behavior with repeatable contract tests.

### Test Layers

#### 1) Unit Tests (fast, required)

- Scope:
  - policy evaluation logic
  - terminal store/session lifecycle
  - connector registry behavior
  - queue payload encoding/decoding helpers
- Target runtime: under 60 seconds total in CI.

#### 2) Service-Level API Tests

- Scope:
  - API handlers for terminal session creation, persistent terminal attach/detach/delete, command queueing, audit listing.
  - Policy evaluation behavior (in-process via `policy.Evaluate`).
  - Connector registry action execution (in-process via `connectorsdk.Registry`).
- Approach:
  - use `httptest` with in-memory stores.
  - inject `policyState` and `connectorRegistry` directly (no HTTP stubs needed).

#### 3) Integration Tests (containerized)

- Scope:
  - API -> Postgres job queue -> worker goroutine command flow (within single `labtether` binary).
  - command completion reflected in API state and audit timeline.
  - policy denial path blocks dangerous commands.
- Tooling:
  - Docker Compose test profile.
  - Bash integration scripts (`scripts/integration-queue-flow.sh`) with environment-driven endpoints.

#### 4) Connector Contract Tests

- Scope:
  - every connector must satisfy SDK interfaces and error contracts.
  - capability declaration and discovery response normalization.
- Approach:
  - shared contract test harness.
  - connector fixtures/mocks for external APIs.

#### 5) End-to-End Smoke Tests

- Scope:
  - bootstrap stack.
  - create terminal session.
  - run command.
  - verify completion + audit event.
  - use `./scripts/desktop-smoke-test.sh --list-targets` to enumerate likely desktop smoke candidates, connected-agent state, advertised `webrtc_available` status, published `webrtc_unavailable_reason`, agent-reported local connector reachability, and currently configured hub collectors before choosing an asset ID.
  - for live desktop runtime validation, use `make desktop-smoke-test` with `LABTETHER_DESKTOP_SMOKE_TARGET=<asset-id>` to preflight the asset/agent connection state, create a desktop session, validate the protocol ticket contract, probe the desktop WebSocket, and delete the session afterward.
  - for iOS VNC black-screen triage on a real device, run `./scripts/capture-vnc-repro.sh --target <asset-id> --banner-file <copied-banner-lines.txt>` immediately after the repro; if the banner includes `trace=ios-...`, the helper will correlate it directly with hub `desktop-agent` log lines.
- Cleanup contract:
  - `make smoke-test` uses a unique fixture token per run and deletes/restores the smoke-created resources on exit so repeated runs do not collide or leave stale state behind.
  - cleanup now covers terminal sessions, incidents, action runs, update plans/update runs, synthetic checks, group profiles, failover pairs, collectors, and retention-setting mutations made during the smoke run.
- Runs:
  - nightly CI and pre-release pipeline.
  - desktop smoke is operator-invoked because it requires a real desktop-capable managed asset and protocol-specific expectations (`LABTETHER_DESKTOP_SMOKE_PROTOCOL`, `LABTETHER_DESKTOP_SMOKE_DISPLAY`, `LABTETHER_DESKTOP_SMOKE_EXPECT_AGENT_VNC`).
  - for agent-backed VNC audio verification, add `LABTETHER_DESKTOP_SMOKE_PROBE_AUDIO=1`; the harness will keep the main desktop stream open long enough to probe the audio sideband before cleanup.
  - if the target is agent-backed or WebRTC-only but the asset is missing from `/agents/connected`, the smoke run now fails during preflight with a direct stale-inventory message instead of surfacing a late stream-time `502`.

#### 6) Native App Validation

Native companion apps (iOS, macOS menu bar agent) are maintained in separate private repos. See each app's own repo for build and test instructions.

### CI Gates (Minimum)

- `gofmt` check.
- `go vet`.
- `go test ./...`.
- web type-check/build (`npm run lint`, `npm run build`).

### Quality Targets (MVP)

- Unit test coverage target for critical packages: >= 70%.
- 100% of policy guardrail rules covered by unit tests.
- Integration test coverage for terminal queue flow: required before release.

### Immediate Next Testing Tasks

- Add API handler tests for `/terminal/sessions` and `/terminal/sessions/{id}/commands`.
- Add worker tests for command execution and result behavior (in-process goroutines).
- Add policy evaluation tests for blocked command patterns (in-process function calls).
- Add connector registry tests for discover/health/action execution paths.

---

## Home Assistant Add-on

> This section covers the Home Assistant add-on packaging/lifecycle path.
> For the custom HA integration (LabTether entities/services in HA), see `integrations/homeassistant/` and `docs/plans/2026-02-22-homeassistant-integration-design.md`.

### Goal

Ship LabTether as a first-party Home Assistant add-on so users can install, update, and operate the product from Home Assistant without manual Docker orchestration.

### Current Implementation Status (Experimental)

- Add-on package exists at `integrations/homeassistant/addon/labtether/`.
- Runtime entrypoint is implemented (`run.sh`) and now:
  - validates add-on options from `/data/options.json`,
  - supports auto-generation and persistence of required secrets (`owner/admin/encryption`),
  - starts local Postgres in-container when `database_url` is not provided,
  - launches the LabTether hub binary with persisted runtime env values.
- Repository metadata scaffold is present at `integrations/homeassistant/addon/repository.yaml`.
- Release automation is available in `.github/workflows/homeassistant-addon-release.yml`:
  - builds/pushes `amd64` + `aarch64` add-on images,
  - generates repository layout + `repo-index.json` + tarball artifact,
  - publishes hosted repository branch `homeassistant-addon-repo`.

### Packaging Model

- Runtime: single add-on container running the LabTether hub binary entrypoint.
- Distribution: Home Assistant add-on repository (private for alpha, public later).
- Network mode: bridge by default, optional host mode only when explicit capability is required.
- Persistence: map add-on data dir to `/data`; supports configurable external Postgres or local bundled Postgres data in `/data/postgres`.

### Proposed Repository Layout

```text
integrations/homeassistant/addon/
  repository.yaml
  labtether/
    config.json
  Dockerfile
    run.sh
    README.md
```

### Required Add-on Config Surface

- `labtether_owner_token` (secret)
- `encryption_key` (base64, 32-byte key)
- `labtether_admin_password`
- `database_url` (optional; default local postgres sidecar policy)
- `tls_mode`
- `auto_generate_credentials`

### Lifecycle

**Install:**
1. User adds LabTether add-on repository.
2. User installs add-on and sets required secrets.
3. Add-on performs preflight checks (token present, encryption key valid, data path writable).
4. Add-on starts local Postgres when needed, then starts hub runtime.

**Start:**
1. Supervisor starts container.
2. Entrypoint resolves/generates runtime credentials and optional local DB bootstrap.
3. Hub process runs migrations at startup and begins API service.
4. Health endpoint gates final ready status.

**Upgrade:**
1. Supervisor pulls new image.
2. Hub startup runs schema migrations with existing backup guardrails.
3. Runtime settings and secrets are reused from `/data`.
4. Post-upgrade health check validates API, queue connectivity, and connector health.

**Stop/Restart:**
- Graceful stop timeout: 30s.
- In-flight command jobs are retried via queue semantics.
- Restart does not clear history or runtime settings.

**Uninstall:**
- User chooses retain/delete data.
- If retain: keep `/data` for rollback reinstall.
- If delete: clear runtime data and local credentials.

### Home Assistant Integration Surface

- Expose add-on status + key LabTether counters as HA sensors:
  - `labtether_services_up`
  - `labtether_assets_online`
  - `labtether_dead_letters_24h`
- Web UI link via Ingress panel.
- Optional service calls: trigger action run, trigger update plan, pause/resume automation modes.

### Security Requirements

- Secrets only in Home Assistant secret fields and encrypted-at-rest inside LabTether.
- No plaintext credential material in logs.
- Validate host keys for managed SSH assets when strict mode is enabled.
- Require local network or Tailscale path for remote operator access in MVP.

### Test Plan

- Unit: add-on config validation, secret bootstrap behavior.
- Integration: install/start/upgrade lifecycle in HA dev container, ingress UI availability, API auth with owner token.
- Soak: 24h restart resilience, queue durability during add-on restart.

### Definition Of Done

- Add-on installs from repo and reaches healthy state on fresh Home Assistant instance.
- Upgrade path preserves data and settings.
- Ingress opens dashboard without separate manual reverse-proxy setup.
- Lifecycle runbook documented for support and operations.

## iOS Performance Guardrails

- The iOS shell now keeps realtime refresh handlers attached only for the visible top-level tab. Hidden Alerts, Devices, and Services tabs no longer keep their own periodic refresh work active in the background.
- Foreground resume work is coalesced into one tracked task. Quick app switches no longer force an immediate websocket teardown/reconnect cycle unless the app stays backgrounded long enough to exceed the keep-warm window.
- iOS Logs live mode now pulls compact payloads from `/logs/query` and avoids combining aggressive event-driven refreshes with short-interval polling.
- Terminal/Desktop reconnect work is lifecycle-bound to the presented screen. If the operator dismisses the remote session, pending reconnect attempts and related helper tasks are cancelled instead of continuing off-screen.

## Validation Tooling Notes (2026-03-22)

- When running Playwright against an already-running local frontend (`PLAYWRIGHT_USE_EXISTING_SERVER=1`), force a single worker. The standalone runtime is stable under serialized route sweeps, but parallel workers can overload the shared mocked-session path and create false negatives.
- The topology e2e harness now mocks both `/api/topology` and `/api/edges` from the same inferred dependency dataset so tree and canvas assertions exercise a consistent relationship graph.
- `security/gosec_allowlist.tsv` is maintained as the reviewed snapshot of current scanner findings. After structural file moves or scanner rule updates, refresh the allowlist in the same change so `make security-gosec` does not fail on stale path drift alone.
- Prefer code-level cleanup for broad false-positive families before expanding the reviewed snapshot. Recent examples: shared best-effort close/remove helpers eliminated the repo's main-tree `G104` bucket, guarded FTP size conversion / Darwin socket-fd checks eliminated the main-tree `G115` bucket, and the March 22 hardening passes drove the reviewed baseline from `184` findings to `0`.
- When a finding is real runtime data rather than a hardcoded secret, arbitrary path, or raw log-injection vector, prefer narrow inline `#nosec` comments on the exact field/callsite over a central allowlist entry. This removed the main-tree `G117`, `G304`, `G706`, `G704`, `G703`, `G705`, `G204`, `G402`, and `G101` buckets while keeping the justification adjacent to the schema field, controlled-path access, bounded runtime log site, or explicit operator opt-in that triggered the scanner.
- `scripts/check-gosec-allowlist.sh` now treats an empty finding set as success and retries once if `gosec` produces invalid/empty JSON. This avoids false-red gates when the scanner emits stderr noise or when the filtered result set is legitimately zero.
- `scripts/check-gosec-allowlist.sh` now defaults its install hint to `gosec v2.25.0`. The March 22 follow-up validated the repo against that newer scanner generation, cleared the newly surfaced `G115`, `G117`, `G118`, `G122`, `G124`, `G702`, `G703`, and `G706` findings, and confirmed the previous ad hoc `package main` SSA-builder warning no longer appears on the upgraded raw scan.
