<div align="center">

# LabTether

**Run your homelab like a real operations platform.**

[![CI](https://github.com/labtether/labtether/actions/workflows/ci.yml/badge.svg)](https://github.com/labtether/labtether/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)](https://docs.docker.com/compose/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square)](LICENSE)

[Website](https://labtether.com) &middot; [Docs](https://labtether.com/docs) &middot; [Wiki](https://labtether.com/docs/wiki) &middot; [Changelog](CHANGELOG.md)

</div>

<!-- TODO: Add hero screenshot of dashboard (dashboard-dark.png) -->

---

## Why LabTether

You run Proxmox, TrueNAS, Docker, maybe Home Assistant. Each has its own dashboard, its own alerts, its own way of telling you something is wrong. When a drive starts failing at 2 AM, you're opening five tabs trying to piece together what happened.

LabTether replaces the tab sprawl with one dashboard, one timeline, one URL.

- **One dashboard** for metrics, logs, alerts, incidents, and actions -- no tab sprawl.
- **Multi-platform** -- manage Linux, Windows, macOS, and FreeBSD from one place.
- **Self-hosted** -- Docker Compose + Postgres. Your data never leaves your network.
- **Integrations** -- Proxmox, TrueNAS, Docker, Portainer, Home Assistant, and more.

---

## Quick Start

Get a full LabTether hub running in under 5 minutes. You need Docker and Docker Compose.

**1. Download the Compose file**

```bash
curl -fsSL https://raw.githubusercontent.com/labtether/labtether/main/deploy/compose/docker-compose.deploy.yml \
  -o docker-compose.yml
```

**2. Configure**

```bash
cat > .env.deploy << 'EOF'
LABTETHER_VERSION=latest
LABTETHER_HUB_IMAGE=ghcr.io/labtether/labtether/labtether:${LABTETHER_VERSION}
LABTETHER_WEB_IMAGE=ghcr.io/labtether/labtether/web-console:${LABTETHER_VERSION}
POSTGRES_IMAGE=postgres:18-alpine
GUACD_IMAGE=guacamole/guacd:1.6.0
EOF
```

**3. Launch**

```bash
docker compose up -d
```

Open **https://localhost:8443** -- TLS certificates are generated on first boot. The setup wizard walks you through the rest.

> Full guide with Tailscale remote access, custom TLS, OIDC SSO, and multi-user setup at [labtether.com/docs](https://labtether.com/docs).

---

## Install Agents

Agents are optional -- connectors like Proxmox and TrueNAS work without them. Agents unlock deeper telemetry, remote terminal and desktop access, and action execution on your nodes.

### Linux

Download the agent binary from [Releases](https://github.com/labtether/labtether-agent/releases/latest) (amd64 or arm64), then enroll:

```bash
curl -fsSL https://github.com/labtether/labtether-agent/releases/latest/download/labtether-agent-linux-amd64 \
  -o /usr/local/bin/labtether-agent
chmod +x /usr/local/bin/labtether-agent
labtether-agent --hub wss://your-hub:8443/ws/agent --enrollment-token YOUR_TOKEN
```

Or use the hub's generated install command -- it handles the download automatically.

See the [Linux agent setup guide](https://labtether.com/docs/install-upgrade/agent-install-commands-by-os) for systemd installation and enrollment.

### macOS

Download **LabTether Agent.app** from [Releases](https://github.com/labtether/labtether-mac/releases/latest). Drag to Applications and launch -- the menu bar icon handles enrollment.

### Windows

Download **LabTether Agent** from [Releases](https://github.com/labtether/labtether-win/releases/latest) and run the installer. The system tray icon handles enrollment.

### FreeBSD

FreeBSD nodes are managed agentlessly via connectors. No agent install required.

> Agent docs: [labtether.com/docs/install-upgrade/agent-install-commands-by-os](https://labtether.com/docs/install-upgrade/agent-install-commands-by-os)

---

## What You Get

<!-- TODO: Add feature screenshots when demo environment is ready -->

**Fleet Dashboard** -- Health at a glance. CPU, memory, disk, network, and temperature across every node.

**Remote Access** -- Terminal and desktop sessions directly from the browser. No SSH keys or VNC clients needed.

**Alerts and Incidents** -- Define rules, get notified, triage and resolve from one timeline with correlated telemetry.

**Integrations** -- Connect what you already run: Proxmox VE, TrueNAS, Docker, Portainer, Home Assistant, and Proxmox Backup Server.

**Update Runs** -- Plan and execute maintenance across your fleet with dry-run support and audit trails.

---

## Supported Integrations

<p align="center">
  <img src="https://img.shields.io/badge/Proxmox%20VE-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="Proxmox VE" />
  <img src="https://img.shields.io/badge/Proxmox%20Backup-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="Proxmox Backup Server" />
  <img src="https://img.shields.io/badge/TrueNAS-0095D5?style=for-the-badge&logo=truenas&logoColor=white" alt="TrueNAS" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/Portainer-13BEF9?style=for-the-badge&logo=portainer&logoColor=white" alt="Portainer" />
  <img src="https://img.shields.io/badge/Home%20Assistant-41BDF5?style=for-the-badge&logo=homeassistant&logoColor=white" alt="Home Assistant" />
</p>

---

## Ecosystem

| | Platform | Description |
|:---|:---------|:------------|
| **[Linux Agent](https://github.com/labtether/labtether-agent)** | Linux | Telemetry, remote access, and actions for Linux machines. |
| **[macOS Agent](https://github.com/labtether/labtether-mac)** | macOS 13+ | Menu bar app with status, enrollment, and notifications. |
| **[Windows Agent](https://github.com/labtether/labtether-win)** | Windows 10+ | System tray app with service management and auto-updates. |
| **[CLI](https://github.com/labtether/labtether-cli)** | Cross-platform | Manage your hub from the terminal. |
| **iOS Companion** | iPhone / iPad | Mobile fleet monitoring and push notifications. |

---

## Documentation

- **User Guide and Wiki** -- [labtether.com/docs](https://labtether.com/docs)
- **Changelog** -- [CHANGELOG.md](CHANGELOG.md)
- **Contributing** -- [CONTRIBUTING.md](CONTRIBUTING.md)
- **Security** -- [SECURITY.md](SECURITY.md)
- **License** -- [Apache 2.0](LICENSE)

---

<div align="center">

Copyright 2026 LabTether. [Apache 2.0](LICENSE)

</div>
