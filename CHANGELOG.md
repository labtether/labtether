# Changelog

All notable changes to LabTether are documented in this file.

This changelog starts at the public-release preparation baseline on 2026-03-09.
The project does not yet publish a long tagged release history, so this file begins
with the current release contract and will grow forward from here.

## [Unreleased]

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
