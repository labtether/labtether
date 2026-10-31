# Agent Packaging Templates

These templates install the LabTether Agent as an OS-native service.

- `systemd/labtether-agent.service`: Linux service unit.
- `launchd/com.labtether.agent.plist`: macOS LaunchDaemon template (headless; for GUI use the menu bar app instead).
- `windows/install-agent.ps1`: Windows service install/update helper (headless; for GUI use the system tray app instead). Pass the enrollment token with `-EnrollmentTokenFile <path>`; the helper stores it in a file only SYSTEM and Administrators can read. It sets the release-signing key (`-UpdatePublicKey`) so the agent can verify self-updates. To trust the Hub's own CA, pass `-CAFingerprintSha256 <fingerprint>` with a `wss://` or `https://` `-HubURL`; the fingerprint is under Settings > TLS in the console.
- `freebsd/labtether_agent`: FreeBSD `rc.d` script template.

All templates expect:
- Binary path: `/usr/local/bin/labtether-agent` (or Windows equivalent).
- Environment values for:
  - `LABTETHER_WS_URL` (hub WebSocket URL)
  - `LABTETHER_API_TOKEN`, or `LABTETHER_ENROLLMENT_TOKEN_FILE` (preferred) or `LABTETHER_ENROLLMENT_TOKEN`
  - `AGENT_ASSET_ID`
  - optional `AGENT_SITE_ID`
  - optional `AGENT_SOURCE` (defaults to `labtether-agent`)
  - optional `LABTETHER_AUTO_UPDATE` (`true|false`, defaults to `true`)
  - `LABTETHER_AUTO_UPDATE_TRUSTED_PUBLIC_KEY` (the key in `scripts/release/agent-release-public-key.b64`) for agents without a built-in key (releases before v1.7.1); without it they refuse every self-update
  - optional `LABTETHER_TLS_CA_FILE` (path to trusted hub CA certificate PEM)
  - optional `LABTETHER_TLS_SKIP_VERIFY` (`true|false`, bootstrap-only bypass for untrusted certs)

## Desktop OSes: Prefer GUI Apps

On **macOS** and **Windows**, prefer the native GUI agent apps over headless daemons:
- **macOS**: `LabTether Agent.app` — menu bar icon with status, settings, notifications. Built from a separate private repo.
- **Windows**: `LabTether Agent.exe` — system tray icon (planned, separate private repo).

The headless daemon templates above are still available for servers or unattended installs.
