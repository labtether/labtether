# Known Issues and Limitations

This page tracks current public-release caveats and the best known workarounds.

## 1. Self-signed TLS trust prompts in `auto` mode

**Affected surface**
- browser access
- iOS/macOS native clients
- first-time local/LAN setups

**What happens**
- Browsers and native clients will not trust the generated LabTether CA automatically.

**Workaround**
- Install or trust the generated CA certificate before broader rollout.
- Use an external certificate if you want a fully trusted deployment from day one.
- References:
  - [Install with Docker Compose](https://labtether.com/docs/wiki/install-upgrade/install-docker-compose)
  - [Security Hardening Checklist](https://labtether.com/docs/wiki/operations/security-hardening-checklist)
  - [Security Posture](https://labtether.com/docs/wiki/reference/security-posture)

## 2. Linux WebRTC desktop depends on host prerequisites

**Affected surface**
- Linux desktop streaming with WebRTC

**What happens**
- WebRTC desktop support can be unavailable when required capture/encoding dependencies are missing.
- In that state, operators should expect fallback to other desktop paths such as VNC where applicable.

**Workaround**
- Install the required desktop prerequisites on the target Linux host and re-run desktop validation.
- References:
  - [Desktop Connection Failures](https://labtether.com/docs/wiki/troubleshooting/desktop-connection-failures)
  - [Protocol Support Matrix](https://labtether.com/docs/wiki/reference/protocol-support-matrix)
  - [Desktop Workflow](https://labtether.com/docs/wiki/core-workflows/desktop-workflow)

## 3. Home Assistant add-on is still experimental

**Affected surface**
- `integrations/homeassistant/addon/labtether/`
- Home Assistant add-on repository packaging path

**What happens**
- The add-on runtime exists and can run the hub, but it is not yet the default production recommendation for public release.

**Workaround**
- Prefer the Docker Compose deployment path for production.
- Use the Home Assistant custom integration for the supported HA-facing integration surface.
- References:
  - [Production Deployment Checklist](https://labtether.com/docs/wiki/operations/production-deployment-checklist)
  - [Supported Release Matrix](https://labtether.com/docs/wiki/reference/supported-release-matrix)
  - [Home Assistant connector docs](https://labtether.com/docs/wiki/connectors/home-assistant)

## 4. Windows and FreeBSD parity still trails Linux and macOS

**Affected surface**
- deeper node-management workflows on Windows and FreeBSD

**What happens**
- Cross-platform behavior is improving, but the deepest current release validation is on Linux and macOS.
- Windows remains parity-track, and FreeBSD is not part of the published agent-release artifact contract yet.

**Workaround**
- Validate required workflows on your target platform before depending on them in production.
- Prefer Linux/macOS for the broadest currently documented and validated depth.
- References:
  - [Supported Release Matrix](https://labtether.com/docs/wiki/reference/supported-release-matrix)
  - [Platform Support](docs/internal/PLATFORM.md)

## 5. UniFi and TP-Link connectors are not in the current release contract

**Affected surface**
- users expecting first-party UniFi or TP-Link connector setup today

**What happens**
- Those connectors are still roadmap work and should not be treated as currently shipped functionality.

**Workaround**
- Use the currently supported connector set instead.
- References:
  - [Supported Release Matrix](https://labtether.com/docs/wiki/reference/supported-release-matrix)
  - [Connector Strategy](https://labtether.com/docs/wiki/connectors/overview)

## Keep This Current

- Add confirmed public-facing limitations here when they materially affect setup, operations, or upgrade decisions.
- Remove entries when the issue is fully resolved and the workaround is no longer needed.
