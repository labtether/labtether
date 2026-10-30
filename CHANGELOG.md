# Changelog

All notable changes to LabTether are documented in this file.

This changelog starts at the public-release preparation baseline on 2026-03-09.
The project does not yet publish a long tagged release history, so this file begins
with the current release contract and will grow forward from here.

## [1.7.1] - unreleased

User-visible changes since v1.4.0. Hub v1.7.0 was never released, so v1.7.1 is
the upgrade from v1.4.0. Hub and agents use wire protocol v1.7.1.

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
  upgrading, back up the unnamed volume, not `postgres-data`. Do not run
  `docker compose pull` on v1.4.0 first: `postgres:18-alpine` images built
  since 21 October 2026 refuse v1.4.0's mount at `/var/lib/postgresql/data`,
  so PostgreSQL does not start.
- **The Hub no longer runs as root.** v1.4.0 ran it as root, so the files it
  wrote (including `/data/install/secrets.json`, `state.json` and the TLS keys
  in `/data/certs`) are owned by root with mode 0600. 1.7.1 runs the Hub as
  `65532:65532`. On every start, the `labtether-bootstrap` job gives
  root-owned files in the Hub's data, CA, service-token and agent-cache
  volumes to that user. It does not follow symlinks or leave the volume.
  Nothing else changes, and v1.4.0 can still read the files if you restore
  it.
- **Old enrollment tokens stop working.** Migration v98 disables every
  enrollment token created before 1.7.1, because those tokens name no device
  or group. Agents that are already enrolled keep their own credentials and
  stay connected. Create a new token for any agent you have not installed yet.
  Do not roll back only the Hub after v98; restore the pre-upgrade database
  backup together with the older Hub.
- **Agents can no longer connect with the shared owner token.** Agent
  WebSockets and legacy HTTP heartbeats that authenticate with the Hub's
  owner token are refused. Set `LABTETHER_ALLOW_LEGACY_SHARED_AGENT_AUTH=true`
  only while you move such an agent to its own enrollment.
- **Viewers are now read-only for host contents.** A `viewer` (session or API
  key, whatever its scopes) can no longer download or list host files, open
  terminal, desktop, Docker-exec or Portainer-exec streams or their tickets,
  read session recordings, process lists, cron entries, host users, host
  journal, Proxmox node syslog, TrueNAS directory listings or container logs,
  the remote clipboard, or use the MCP tools
  `files_list`, `files_read`, `system_processes` and `docker_container_logs`.
  These now return `403 operator role required`. Inventory, status, alerts
  and the central log view stay readable. Give such users the `operator` role.
- **Admin-only actions:** approving pending agents (`/api/v1/agents/approve`,
  `/api/v2/agents/pending/approve`) and changing retention settings now need
  `admin` or `owner`. Audit retention cannot go below 30 days; stored shorter
  values are raised to 30 days.
- **A Hub with no encryption key will not start over encrypted data.** If
  the database holds saved credentials, webhook secrets or 2FA enrolments but
  `secrets.json` has no key and `LABTETHER_ENCRYPTION_KEY` is not set, the Hub
  refuses to start (`encryption_configuration`) instead of making a new key
  that cannot read them. Restore `secrets.json` from the same backup as the
  database. `scripts/db-backup.sh` covers the database only and now says so.
- **`LABTETHER_ENCRYPTION_KEY` must match the stored key.** If it differs from
  the key in `$LABTETHER_DATA_DIR/install/secrets.json`, the Hub refuses to
  start (`encryption_configuration`) instead of silently making saved
  credentials unreadable. Unset the variable or set it to the stored value.
  To deliberately discard data encrypted with the old key, start once with
  `LABTETHER_ENCRYPTION_KEY_REPLACE=true`.
- **A restored database needs the key it was written with.** The database
  now keeps a check value encrypted with the Hub's key. If the Hub's key
  cannot read it, for example after restoring a database onto a Hub with its
  own `secrets.json`, the Hub refuses to start (`encryption_configuration`)
  and logs the ID of the key the database needs and of its own key. The first
  start after the upgrade writes the check, so a mismatch that already exists
  is not caught. `LABTETHER_ENCRYPTION_KEY_REPLACE=true` discards the old
  secrets and rewrites the check.
- **The Hub SSH key is set per device.** Devices that exist at the upgrade
  keep the Hub's root SSH key; devices added later start without it. Admins
  turn it on or off on the device's Connect panel
  (`/assets/{id}/hub-ssh-key`). Turning it off removes the key through the
  agent. Uninstall and purge remove every `labtether-hub` key line.
- **Linux agent installer:** `install.sh` no longer adds the Hub CA to the
  system trust store; purge removes a matching copy left by older installers,
  and the `labtether` helper. The systemd unit no longer sets
  `ProtectHome=read-only`, so Files, terminal writes and the Hub SSH key work
  in `/root` and `/home`. Plain-HTTP and `localhost` Hub URLs get the agent
  settings they need to connect. Docker and file-scope settings go into
  `agent.env` only when you pass their flag, so the Hub manages them
  otherwise.
- **SSO roles: no IdP group grants admin by default.** The admin group list
  (`LABTETHER_OIDC_ADMIN_ROLES`, or Admin Role Values under Security → OIDC /
  SSO) used to default to `admin`, so members of any IdP group named `admin`
  became LabTether admins. It is now empty, and a group named `admin` or
  `owner` grants nothing by its name. Before upgrading, put the names of the
  groups that should grant admin in that list, or send a `labtether_role`
  claim; otherwise those users drop to the default role at their next
  sign-in. Saving the SSO settings in the console before 1.7.1 stored the
  prefilled `admin` in the list; remove it if that group should not grant
  admin.
- **API keys follow their creator:** a key's role is capped at its creator's
  current role, and keys whose creator was deleted stop working. Keys made
  with the owner token keep their role.
- **Console token file:** `/run/labtether/api-token` now holds a token only
  the console can use, not the API token. Scripts that sent it as an owner
  credential get `403`; give them an API key.
- **Admin accounts:** only the owner can change, sign out or delete an admin.
  An edited schedule runs with the editor's rights. Action run history
  (`/actions/runs`) needs `operator` and shows a key only its allowed devices.
- **Webhooks:** a new or changed `http://` webhook URL is refused unless
  `LABTETHER_ALLOW_INSECURE_TRANSPORT=true`. Viewers see only the scheme and
  host of webhook URLs.
- **Wake-on-LAN from a container:** with no online agent on the device's
  network, a Hub in a bridged container returns `409` instead of reporting a
  packet that never left the container. On macvlan networking, set
  `LABTETHER_WOL_HUB_BROADCAST=true`.
- **Command allowlist rules:** a rule that starts with `=` must match the
  whole command. Rules for `cmd`, `powershell`, `pwsh`, `sh`, `bash` and other
  command interpreters always need a whole-command match, because extra
  arguments to them run as code. Check custom `COMMAND_ALLOWLIST_PREFIXES`
  lists.
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
- **Slow first start after an upgrade:** the deploy Compose file now gives
  the Hub up to 30 minutes (the default migration time limit) to become
  healthy before Compose gives up on it, and checks every 5 seconds meanwhile.
  Before, a migration longer than about 45 seconds left the console stopped.
- **Uploaded TLS certificates:** the Hub keeps serving its built-in
  certificate for names the uploaded one does not cover, such as the
  `labtether` name the bundled console uses. Before, uploading a certificate
  cut the console off from the Hub.
- **Release asset renamed:** the deploy env template is published as
  `deploy.env.example` (GitHub renamed `.env.deploy.example` to
  `default.env.deploy.example`). `SHA256SUMS` lists `deploy-manifest.json`,
  `deploy.env.example` and `docker-compose.deploy.yml`.
- **Image tags:** a release tags `ghcr.io/labtether/labtether:<version>` only
  after the pushed image has started the full deploy stack, console included.
  `:latest` and the GitHub latest release move only for the highest `vX.Y.Z`
  tag, so a patch for an older line no longer takes them over.
- **Login limits:** each client gets 30 login attempts per minute before the
  shared 120-per-minute budget. Behind your own reverse proxy, set
  `LABTETHER_TRUST_PROXY_CIDRS` or `LABTETHER_TRUST_PROXY_HOSTS` so the
  forwarded client address is used. Through the deploy Compose console each
  browser has its own limit. The all-in-one image has no console ingress, so
  console sign-ins there share one limit unless `LABTETHER_TRUST_PROXY_HOPS`
  names your proxy.
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
- Terminal, remote view, container shell and event streams close when the
  sign-in session, user, role or API key behind them ends. Stream tickets are
  checked again when used.
- SSO sign-in finishes only in the browser that started it, and the login
  page shows only its own error messages. No IdP group grants admin unless
  it is in the admin group list, which is now empty by default.
- Container shell output and agent log streams are no longer sent to every
  console and webhook. Docker events name the agent that sent them.
- An agent can no longer pick the Hub address it moves to after enrolling, or
  the host the Hub's SSH key is used on. New Hub CAs sign server certificates
  only.
- Home Assistant attributes holding tokens or passwords are not stored.
  Redirects to another host drop connector API key headers.
- Unauthenticated discover lists Hub addresses only to local and tailnet
  callers.
- The console opens only `http` and `https` service links.
- Audit events name who changed users, passwords, 2FA and OIDC settings, who
  ran an admin reset or a multi-host command, and who read a saved desktop
  password. Admin reset keeps the audit log.

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
- Fresh installs from the deploy Compose file start the console: the Hub
  health check and console ingress run Node from `/usr/local/bin`.
- Agents stay enrolled past the token lifetime; each heartbeat extends the
  agent's token.
- Install-time agent settings such as `--docker-enabled true` are no longer
  replaced by Hub defaults when the agent connects, and the device's agent
  settings show the value the agent runs with.
- Container IDs from `labtether-cli docker ps` work for container logs,
  stats, start, stop and restart.
- `install.sh --purge` and `labtether agent purge` remove the Hub CA from the
  system trust store and the `labtether` helper.
- The Hub's default command allowlist includes read-only Windows commands:
  `whoami`, `hostname`, `ipconfig`, `ipconfig /all`, `systeminfo`,
  `tasklist`, `cmd /c ver` and `powershell -Command Get-Date`. Agents older
  than 1.7.1 refuse them with their own allowlist.
- A device delete refused because its agent is connected no longer removes the
  Hub SSH key from that device first.
- The Linux installer's final check asks the agent on the port its service
  uses, not always 8090.
- SSO sign-in works on a plain-HTTP console.
- MCP reports the Hub's version instead of `1.0.0`.
- File uploads and downloads time out only when they stall. Staging files go
  to the data volume (`/data/spool`), not memory-backed `/tmp`.
- Agent file downloads start while the file is still arriving, so a download
  that takes more than 5 minutes no longer fails through the console.
- Home Assistant shows every entity, not the first 40.
- Each webhook has its own delivery queue, so one slow endpoint no longer
  delays or drops events for the others.
- Retention prunes in batches and carries on past a slow table. Pausing an
  alert rule resolves its open alerts.
- The Add Device Linux command passes Docker and file-scope flags only when
  you change them.
- `deploy/agents/windows/install-agent.ps1` sets the release-signing key, so
  agents it installs can self-update.
- Command results keep the device's real exit code instead of a made-up 0
  or 1. A command that exits 2 reports 2 in the v2 exec API (single and
  fleet), bulk service actions, MCP exec and saved action results, and is
  stored with terminal commands and action run steps (migration 108). When
  the code is unknown (an older agent, a timeout, a command that never ran)
  the API leaves `exit_code` out and `status` says `succeeded` or `failed`;
  the console shows Succeeded or Failed. The code itself needs agent 1.7.1.

### Changed

- Bounded browser event delivery, remote-write serialization, job-queue
  notification batching and passive console polling reduce Hub load.
- Agent file downloads have no `Content-Length`. A download that fails after
  it started ends with a network error instead of a JSON error.

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
