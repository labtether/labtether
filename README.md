# LabTether

[![CI](https://github.com/labtether/labtether/actions/workflows/ci.yml/badge.svg)](https://github.com/labtether/labtether/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)
![Hub](https://img.shields.io/badge/Hub-Docker%20%2B%20Postgres-2496ED?logo=docker&logoColor=white)
![Remote Access](https://img.shields.io/badge/Remote-Tailscale%20Recommended-0A84FF)

Run your homelab like a real operations platform.

LabTether is an all-in-one homelab control plane for observability, operations, and automation.
Monitor health, investigate incidents, run actions, and orchestrate updates from one dashboard.

[Start In 5 Minutes](#quick-start-about-5-minutes) • [Architecture At A Glance](#architecture-at-a-glance) • [Use Cases](#use-cases-by-operator-type) • [Documentation](https://labtether.com/docs)

![LabTether Dashboard](./dashboard-dark.png)

## Why LabTether

- One dashboard for metrics, logs, alerts, incidents, actions, and updates.
- Faster triage with correlated telemetry and change history.
- Safer operations with policy-aware execution and audit trails.
- Multi-platform management across Linux, Windows, macOS, and FreeBSD.
- Self-hosted architecture with Docker Compose + Postgres.
- No required Grafana/Prometheus/Loki workflow for day-to-day operations.

## Who It's For

- Homelab operators running mixed infrastructure (compute, storage, network, smart home).
- Builders who want one control plane instead of dashboard sprawl.
- Teams/families sharing responsibility for home infrastructure reliability.

## What You Can Do Today

- Observe fleet health with inventory, telemetry, logs, alerts, incidents, and reliability views.
- Create alert rules from shared backend templates or custom conditions directly in the console.
- Run remote terminal sessions and connector-native actions from the same console.
- Plan and execute maintenance with update runs, synthetic checks, and retention controls.
- Integrate core homelab systems: Proxmox, TrueNAS, Docker, Portainer, Home Assistant, and PBS.

## Current Release Scope

- Auth: built-in owner access, local managed users with `owner`/`admin`/`operator`/`viewer` roles, and optional OIDC SSO.
- Current connector and collector surface: Proxmox VE, Proxmox Backup Server, TrueNAS, Docker, Portainer, and Home Assistant.
- Native companions: iOS companion app and macOS menu bar agent are available separately (see [labtether.com](https://labtether.com)).
- Experimental packaging: Home Assistant add-on runtime and repository publishing path.
- Not part of the current public-release contract: UniFi and TP-Link connectors remain planned roadmap work.

## Real Workflow Wins

- Diagnose outages faster with a single timeline for logs, metrics, alerts, and actions.
- Execute maintenance safely with preplanned update runs and auditable change history.
- Operate mixed nodes and integrations from one URL and one operator experience.

## Use Cases By Operator Type

| Operator | Daily Friction | How LabTether Helps |
|---|---|---|
| Solo homelab owner | Too many tabs, no single source of truth during incidents | Correlates telemetry, logs, alerts, and actions in one triage workflow |
| Shared household/team operator | Handoffs are inconsistent and changes are hard to track | Provides one dashboard plus audit trails for safer shared operations |
| Multi-lab consultant | Context-switching between environments is expensive | Standardizes operations and maintenance patterns across labs |

## Platform Model

- **Hub**: Docker-hosted LabTether control plane.
- **Node**: Managed endpoint asset.
- **Agent**: Optional node-local helper for deeper telemetry/execution. Agentless management remains supported.

## Architecture At A Glance

![LabTether Architecture Overview](./docs/internal/assets/architecture-overview.svg)

LabTether keeps the control plane simple: one hub service, one Postgres backend, one web console, optional agents, and connector integrations.

## Supported Systems

| Area | Current |
|---|---|
| Hub runtime | Docker + Postgres |
| Node platforms | Linux, Windows, macOS, FreeBSD |
| Connectors | Proxmox, TrueNAS, Docker, Portainer, Home Assistant, PBS |
| Web console | Next.js console at `http://localhost:3000` |
| Auth model | Local auth + roles (`owner/admin/operator/viewer`) + optional OIDC SSO + owner token |

## Quick Start (About 5 Minutes)

1. Install with the release-image deploy path:
```bash
./scripts/install-compose.sh --version vX.Y.Z
```

This command creates `.env.deploy` when you are running from a repository checkout, pins the requested LabTether release version, pulls published images, and starts the split-Postgres deploy stack.

LabTether runtime secrets are generated automatically on first boot and persisted in the LabTether data volume. The managed Postgres password is generated automatically by the deploy stack and can be revealed later from `Settings` by an admin.

2. Open:
- Console: `http://localhost:3000`
- API health: `http://localhost:8080/healthz`

3. Fresh local database sign-in:
- Username: `admin` by default
- If `LABTETHER_ADMIN_PASSWORD` is blank, finish setup at `http://localhost:3000/setup`
- If `LABTETHER_ADMIN_PASSWORD` is set, use that value from `.env.deploy`
- Optional: configure `LABTETHER_OIDC_*` vars to enable `Sign in with SSO`.

## Manual Startup (Advanced)

If you are using a tagged release artifact, the default split-Postgres install can run without an env file:

```bash
docker compose -f docker-compose.deploy.yml up -d
```

Add `.env.deploy` only when you want overrides such as bind addresses, admin bootstrap values, or profile toggles.

If you are running from a local repository checkout, use the repo-managed env file:

1. Configure environment:
```bash
cp .env.deploy.example .env.deploy
# set LABTETHER_VERSION
# review image pins and optional overrides
```
2. Start LabTether:
```bash
docker compose --env-file .env.deploy -f docker-compose.deploy.yml up -d
```
3. Validate:
```bash
docker compose --env-file .env.deploy -f docker-compose.deploy.yml ps
```
4. Open:
- Console: `http://localhost:3000`
- API health: `http://localhost:8080/healthz`

First-run auth behavior:
- `LABTETHER_ADMIN_USERNAME` defaults to `admin`
- if `LABTETHER_ADMIN_PASSWORD` is blank, complete setup in the browser at `http://localhost:3000/setup`
- if `LABTETHER_ADMIN_PASSWORD` is set, that account is created during startup

Managed install state:
- owner token, API token, and encryption key are generated automatically and persisted under the LabTether data volume
- the split-Postgres password is generated automatically and can be revealed later by an admin in `Settings`

To upgrade later:
```bash
./scripts/upgrade-compose.sh --version vX.Y.Z
```

## Interface Preview

| Dashboard | Alerts | Settings |
|---|---|---|
| ![Dashboard Light](./dashboard-light.png) | ![Alerts](./alerts-dark.png) | ![Settings](./settings-dark.png) |

## Local Development

Default dev workflow (tmux background sessions):

```bash
make dev-up
# when you want a clean refresh:
make dev-up-restart
# when you want to stop just one side:
make dev-backend-stop
make dev-frontend-stop
```

Contributor/bootstrap path for integration-like local Docker validation:

```bash
make bootstrap
```

Core checks:

```bash
make fmt
make lint
make test
cd web/console && npx tsc --noEmit
```

Runtime smoke checks:

```bash
./scripts/desktop-smoke-test.sh --list-targets
make smoke-test
LABTETHER_DESKTOP_SMOKE_TARGET=<asset-id> make desktop-smoke-test
LABTETHER_DESKTOP_SMOKE_TARGET=<asset-id> LABTETHER_DESKTOP_SMOKE_EXPECT_AGENT_VNC=1 LABTETHER_DESKTOP_SMOKE_PROBE_AUDIO=1 make desktop-smoke-test
```

`./scripts/desktop-smoke-test.sh --list-targets` prints the current likely desktop smoke candidates with source, platform, status, connected-agent state, advertised `webrtc_available` status, any published `webrtc_unavailable_reason`, agent-reported local connector reachability, and a summary of configured hub collectors. If the stack has no connector-backed desktop assets yet, it will say so directly. `make desktop-smoke-test` then preflights the target asset and `/agents/connected` before opening a session, so agent-backed and WebRTC smoke runs fail fast when inventory says a node exists but its agent is no longer actually connected or does not currently advertise real WebRTC support.

## Security Posture (Current)

- Private-by-default self-hosted deployment.
- Tailscale-only remote access is the recommended path.
- Protected API endpoints require bearer token auth.
- Secrets/credentials are encrypted at rest.
- Operational actions are audit logged.

## Docs

- [Internal Documentation](docs/internal/README.md)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)
- [Known Issues](KNOWN_ISSUES.md)
- [Support](SUPPORT.md)
- [Privacy](PRIVACY.md)
- [Security Reporting](SECURITY.md)
- [User Guide](https://labtether.com/docs)
- [Operator Wiki](https://labtether.com/docs)
- [Solo Operations Guide](docs/internal/SOLO_OPERATIONS.md)
- [Remote Access (Tailscale-First)](https://labtether.com/docs/wiki/reference/remote-access)
- [API Reference](https://labtether.com/docs/wiki/reference/api)
- [Architecture](docs/internal/ARCHITECTURE.md)
- [Architecture Decision Records](docs/internal/ADR.md)

## Roadmap Focus

- Canonical data model rollout across all connector sources.
- Continued Linux/macOS/Windows/FreeBSD capability parity improvements.
- Deeper reliability automation and incident workflows.
- Additional connector coverage for network edge, storage, and orchestration ecosystems.

## Monorepo Layout

- `cmd/labtether/` - single-binary hub service.
- `services/migrator/` - standalone DB migration tool.
- `agents/` - optional endpoint helpers (`labtether-agent`).
- `web/console/` - operator web console.
- `internal/` - shared domain modules and runtime packages.
- `deploy/` - Docker and infra configs.
- `docs/internal/` - internal architecture and operations references.
- `notes/` - active execution tracking.

## Contributing

Contributions are welcome. Before opening a PR:

1. Review active priorities in `notes/TODO.md`.
2. Read architectural decisions in `docs/internal/ADR.md`.
3. Run `go vet ./...`, `go test ./...`, and `cd web/console && npx tsc --noEmit`.
