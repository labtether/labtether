# LabTether Upgrade Guide

This guide provides version-specific upgrade notes. Always follow the general upgrade procedure in [UPGRADING.md](UPGRADING.md) first.

## General Upgrade Steps

1. Back up: `make db-backup`
2. Check status: `make db-migrate-status`
3. Upgrade: `make upgrade-compose VERSION=v2026.X`
4. Verify: Check hub logs for successful migration and startup

## Version-Specific Notes

### v2026.2

- **Agent distribution:** Agents are now downloaded from GitHub Releases instead of being bundled with the hub. The hub Docker image includes agent binaries at `/opt/labtether/agents/`.
- **New env var:** `LABTETHER_AGENT_MANIFEST_REFRESH` — set to `true` to enable runtime agent cache refresh from GitHub.
- **New env var:** `LABTETHER_AGENT_CACHE_DIR` — runtime agent cache directory (default: `/data/agents`).
- **New volume:** `labtether-agent-cache` mounted at `/data/agents` for persistent agent binary cache.
- **Removed env var:** `LABTETHER_AGENT_RELEASE_VERSION` — replaced by `agent-manifest.json`.
- **New API endpoints:** `GET /api/v1/agent/manifest`, `POST /api/v1/agent/cache/refresh`.
- **Database migrations:** v73 (audit_events_indexes) — adds indexes for audit event queries. No downtime required.

### v2026.1

- Initial public release.
- 73 database migrations applied on first startup.
- First startup may take 30-60 seconds for schema creation.

## Agent Compatibility

| Hub Version | Go Agent | macOS Agent | Windows Agent |
|-------------|----------|-------------|---------------|
| v2026.2 | v2026.2 | v2026.1 | — |
| v2026.1 | v2026.1 | v2026.1 | — |

Agents are backward-compatible within the same major version. Upgrade the hub first, then agents.
