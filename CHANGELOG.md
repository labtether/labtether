# Changelog

All notable changes to LabTether are documented in this file.

This changelog starts at the public-release preparation baseline on 2026-03-09.
The project does not yet publish a long tagged release history, so this file begins
with the current release contract and will grow forward from here.

## [1.7.0] - unreleased

User-visible changes since v1.4.0. Hub and agents use wire protocol v1.7.0.

### Upgrade notes

Read these before upgrading; several change who can do what or need a
configuration step.

- **Upgrade Docker installs with the installer or `scripts/upgrade-compose.sh`,
  not a bare `docker compose up`.** v1.4.0 mounted `postgres-data` at
  `/var/lib/postgresql/data`, but PostgreSQL 18 keeps its database in
  `/var/lib/postgresql/18/docker`, so v1.4.0 databases live in the postgres
  container's unnamed volume and `postgres-data` is empty. Both upgrade paths
  stop the stack and copy that database into `postgres-data` once; the old
  volume is left as a backup. If the database is missing from `postgres-data`,
  the `labtether-bootstrap` job now stops the stack instead of letting
  PostgreSQL create an empty database. If you already ran `docker compose up`
  with the new files, find the old volume with
  `docker volume ls --filter dangling=true` and re-run the upgrade with
  `LABTETHER_POSTGRES_LEGACY_VOLUME=<that volume>`. To start empty on purpose,
  set `LABTETHER_POSTGRES_NEW_DATABASE=true` in `.env.deploy` once. Before
  upgrading, back up the unnamed volume, not `postgres-data`.
- **Viewers are now read-only for host contents.** A `viewer` (session or API
  key, whatever its scopes) can no longer download or list host files, open
  terminal, desktop, Docker-exec or Portainer-exec streams or their tickets,
  read session recordings, process lists, cron entries, host users, host
  journal or container logs, the remote clipboard, or use the MCP tools
  `files_list`, `files_read`, `system_processes` and `docker_container_logs`.
  These now return `403 operator role required`. Inventory, status, alerts
  and the central log view stay readable. Give such users the `operator` role.
- **Admin-only actions:** approving pending agents (`/api/v1/agents/approve`,
  `/api/v2/agents/pending/approve`) and changing retention settings now need
  `admin` or `owner`. Audit retention cannot go below 30 days; stored shorter
  values are raised to 30 days.
- **`LABTETHER_ENCRYPTION_KEY` must match the stored key.** If it differs from
  the key in `$LABTETHER_DATA_DIR/install/secrets.json`, the Hub refuses to
  start (`encryption_configuration`) instead of silently making saved
  credentials unreadable. Unset the variable or set it to the stored value.
  To deliberately discard data encrypted with the old key, start once with
  `LABTETHER_ENCRYPTION_KEY_REPLACE=true`.
- **First admin in development:** without `LABTETHER_ADMIN_PASSWORD`, no admin
  is created with an unseen random password any more. Open `/setup` and enter
  the one-time token from `$LABTETHER_DATA_DIR/install/setup-token` (the log
  names the path), as in production.
- **Portainer collectors verify TLS by default.** Collectors without an explicit
  `skip_verify` now verify the server certificate. For a self-signed Portainer,
  trust its CA or set `skip_verify` on the collector.
- **WebDAV uses HTTPS by default.** Connections saved by older releases that
  used plain HTTP (no `webdav_tls`, port other than 443) now fail with `409` and
  an explanation, and the connection list marks them with `transport_notice`.
  Edit the connection to use HTTPS, or keep plain HTTP by setting
  `webdav_allow_cleartext=true` on the connection and
  `LABTETHER_ALLOW_INSECURE_TRANSPORT=true` on the Hub. Plain HTTP is never
  re-enabled automatically.
- **Session recordings live on the data volume:** new recordings go to
  `$LABTETHER_DATA_DIR/recordings`. Files written by older releases to the
  working directory's `data/recordings` stay downloadable while that directory
  exists; copy them into the data volume to keep them across container
  replacement.
- **Long migrations:** schema migrations run without the 30 s statement
  timeout. The whole run is bounded by `LABTETHER_DB_MIGRATION_TIMEOUT`
  (default `30m`) and each lock wait by `LABTETHER_DB_MIGRATION_LOCK_TIMEOUT`
  (default `5m`). The first start on a large `metric_samples` table can take
  several minutes (migration v88 builds an index).
- **File transfers** left pending or in progress when the Hub stopped are
  marked failed at the next start.
- **Removed `LABTETHER_AGENT_MANIFEST_REFRESH`**; no code read it.
  `POST /api/v1/agent/cache/refresh` now answers `status: "reloaded"`,
  `source: "disk"` and the loaded agent versions.
- **Release asset renamed:** the deploy env template is published as
  `deploy.env.example` (GitHub renamed `.env.deploy.example` to
  `default.env.deploy.example`). `SHA256SUMS` lists `deploy-manifest.json`,
  `deploy.env.example` and `docker-compose.deploy.yml`.
- **Login limits:** each client gets 30 login attempts per minute before the
  shared 120-per-minute budget. Behind your own reverse proxy, set
  `LABTETHER_TRUST_PROXY_CIDRS` or `LABTETHER_TRUST_PROXY_HOSTS` so the
  forwarded client address is used.
- **Agent auto-update:** `install.sh` now writes the Hub's release-signing key
  (`LABTETHER_AUTO_UPDATE_TRUSTED_PUBLIC_KEY`) to `/etc/labtether/agent.env`.
  Agents installed earlier need the installer re-run (or that line added)
  before signed auto-updates can succeed.

### Security

- Role floor for host data and interactive sessions, enforced in the Hub auth
  middleware for sessions, API keys and stream tickets, and in MCP.
- Portainer collector TLS verification on by default; exact SSH host-key
  matching and bounded SSH handshakes; WebDAV TLS policy.
- Encryption-key replacement guard, 30-day audit floor, admin-only agent
  approval and retention changes, per-client login limiting.
- Connector actions check maintenance windows and authorization at admission
  and at dispatch; agent-reported services bind to the authenticated
  connection's asset; asset-restricted keys no longer see PBS task details.
- API key scope boundaries on legacy routes, secure session cookies, console
  auth request body limits, hardened agent binary downloads.

### Added

- `GET /recordings/{id}/download` streams a finished recording as an
  attachment (operator and above, own recordings unless admin); list entries
  carry `download_url`.
- Auto-TLS serves the Hub CA with the leaf certificate, and the CA stays the
  same across leaf renewals, so clients that pin the CA (iOS app) keep trusting
  the Hub after renewals.
- `LABTETHER_DB_MIGRATION_TIMEOUT` and `LABTETHER_DB_MIGRATION_LOCK_TIMEOUT`.

### Fixed

- Upgrades no longer fail when a migration outlasts 30 s.
- One stalled agent socket or slow browser no longer stalls routing for every
  agent; slow desktop viewers are closed after a 1 s bounded wait.
- A panic in a file-transfer goroutine fails that transfer instead of the Hub.
- Outbound HTTP (connectors, notifications, telemetry) reuses connection pools
  instead of opening a new pool per request.
- Remote control: exact stream ownership and reconnects for agent terminal,
  SSH, desktop, audio, WebRTC and Docker exec streams; deleting a session
  closes every stream it owns; console viewer clipboard and pointer fixes.
- Alert evaluation, firing scope, incident correlation and notification
  pagination; file transfers fail promptly when the agent disconnects or the
  request is cancelled.
- v2 agent response envelopes, bundled console event proxy trust, database URI
  handling, IPv6 redirect targets and Home Assistant quick-config origin.
- Clean image builds after Alpine package updates.

### Changed

- Bounded browser event delivery, remote-write serialization, job-queue
  notification batching and passive console polling reduce Hub load.

## [Unreleased]

### Security
- Admin users can no longer reset the Owner account's password via `PATCH /auth/users/{id}`; the handler rejects password-change requests targeting the owner unless the caller is the owner themselves.
- `POST /auth/me/password` (self-service password change, current-password verified) and `DELETE /auth/account` (self-delete, owner-exempt) are now registered on the HTTP mux; the handlers existed but the routes were not wired, causing the web console's account settings to 404.
- Agent release manifest now plumbs an optional per-binary `signature` field end-to-end. `agent-manifest.json` and `GET /api/v1/agent/releases/latest` both carry the signature when the upstream agent release includes one, so agents configured with `LABTETHER_AUTO_UPDATE_TRUSTED_PUBLIC_KEY` can verify updates without contacting GitHub directly.

### Added

- Repository license published as Apache-2.0.
- Role-based local auth with `owner`, `admin`, `operator`, and `viewer` roles.
- Optional OIDC SSO with role mapping and auto-provisioning.
- Current supported connector surface for Proxmox VE, Proxmox Backup Server, TrueNAS, Docker, Portainer, and Home Assistant.
- Native companion surfaces for the iOS console app and macOS menu bar agent.
- Operator wiki coverage for install, upgrade, workflows, troubleshooting, release readiness, and physical-device validation.
- Public release-support docs: `KNOWN_ISSUES.md`, `SUPPORT.md`, `PRIVACY.md`, and the repo security-reporting policy.

### Changed

- Canonical docs now define a single public release contract across the root README, product docs, PRD, security docs, connector docs, and release-readiness checklist.
- Release readiness now explicitly requires public-facing release artifacts such as changelog/release notes, known issues, support guidance, and a security reporting path.
- Restored credential-profile compatibility for Proxmox username/password onboarding so the console can save `proxmox_password` credentials again.

### Notes

- The Home Assistant custom integration is in current release scope.
- The Home Assistant add-on runtime remains experimental.
- UniFi and TP-Link connectors remain planned roadmap work rather than current public-release scope.
