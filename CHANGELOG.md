# Changelog

All notable changes to LabTether are documented in this file.

This changelog starts at the public-release preparation baseline on 2026-03-09.
The project does not yet publish a long tagged release history, so this file begins
with the current release contract and will grow forward from here.

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
