# LabTether

[![CI](https://github.com/labtether/labtether/actions/workflows/ci.yml/badge.svg)](https://github.com/labtether/labtether/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**Run your homelab like a real operations platform.**

<!-- TODO: Add hero screenshot (dashboard-dark.png) -->

---

## Why LabTether

- **One dashboard** for metrics, logs, alerts, incidents, and actions — no tab sprawl.
- **Multi-platform** — manage Linux, Windows, macOS, and FreeBSD from one place.
- **Self-hosted** — Docker Compose + Postgres. Your data never leaves your network.
- **Integrations** — Proxmox, TrueNAS, Docker, Portainer, Home Assistant, and more.

---

## Quick Start

Get a full LabTether hub running in under 5 minutes.

### 1. Create a `.env.deploy` file

```bash
cat > .env.deploy << 'EOF'
LABTETHER_VERSION=latest
LABTETHER_HUB_IMAGE=ghcr.io/labtether/labtether/labtether:${LABTETHER_VERSION}
LABTETHER_WEB_IMAGE=ghcr.io/labtether/labtether/web-console:${LABTETHER_VERSION}
POSTGRES_IMAGE=postgres:16-alpine
POSTGRES_DB=labtether
POSTGRES_USER=labtether
GUACD_IMAGE=guacamole/guacd:1.6.0
EOF
```

### 2. Download the Compose file

```bash
curl -fsSL https://raw.githubusercontent.com/labtether/labtether/main/docker-compose.deploy.yml -o docker-compose.yml
```

### 3. Start LabTether

```bash
docker compose up -d
```

Open **https://localhost:8443** to access the console. The hub generates TLS certificates automatically on first boot.

> **Full setup guide:** [labtether.com/docs](https://labtether.com/docs) — covers Tailscale remote access, custom TLS, agent enrollment, and more.

---

## Install Agents

Agents are optional — they enable deeper telemetry, remote terminal/desktop access, and action execution on your nodes.

### Linux

```bash
curl -fsSL https://github.com/labtether/labtether-linux/releases/latest/download/labtether-agent-linux-amd64 -o /usr/local/bin/labtether-agent
chmod +x /usr/local/bin/labtether-agent
```

See the [Linux agent setup guide](https://labtether.com/docs/wiki/agents/linux) for systemd installation and enrollment.

### macOS

Download **LabTether Agent.app** from [Releases](https://github.com/labtether/labtether-mac/releases/latest) — a menu bar app with status, settings, and notifications.

### Windows

Download **LabTether Agent** from [Releases](https://github.com/labtether/labtether-win/releases/latest) — a system tray app with service management and auto-updates.

### FreeBSD

FreeBSD nodes are managed agentlessly via connectors. No agent install required.

> **Agent docs:** [labtether.com/docs/wiki/agents](https://labtether.com/docs/wiki/agents)

---

## What You Get

<!-- TODO: Add feature screenshots when demo is ready -->

**Fleet Dashboard** — Health at a glance. CPU, memory, disk, network, and temperature across every node.

**Remote Access** — Terminal and desktop sessions directly from the browser. No SSH keys or VNC clients needed.

**Alerts & Incidents** — Define rules, get notified, triage and resolve from one timeline with correlated telemetry.

**Integrations** — Connect what you already run: Proxmox VE, TrueNAS, Docker, Portainer, Home Assistant, and Proxmox Backup Server.

**Update Runs** — Plan and execute maintenance across your fleet with dry-run support and audit trails.

---

## Supported Integrations

![Proxmox](https://img.shields.io/badge/Proxmox-E57000?logo=proxmox&logoColor=white)
![TrueNAS](https://img.shields.io/badge/TrueNAS-0095D5?logo=truenas&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
![Portainer](https://img.shields.io/badge/Portainer-13BEF9?logo=portainer&logoColor=white)
![Home Assistant](https://img.shields.io/badge/Home%20Assistant-41BDF5?logo=homeassistant&logoColor=white)

---

## Companion Apps

| App | Platform | Link |
|-----|----------|------|
| **iOS Companion** | iPhone / iPad | [labtether.com](https://labtether.com) |
| **CLI** | Linux / macOS / Windows | [labtether/labtether-cli](https://github.com/labtether/labtether-cli) |
| **Linux Agent** | Linux | [labtether/labtether-linux](https://github.com/labtether/labtether-linux) |
| **macOS Agent** | macOS 13+ | [labtether/labtether-mac](https://github.com/labtether/labtether-mac) |
| **Windows Agent** | Windows 10+ | [labtether/labtether-win](https://github.com/labtether/labtether-win) |

---

## Documentation

- **User Guide & Wiki** — [labtether.com/docs](https://labtether.com/docs)
- **Changelog** — [CHANGELOG.md](CHANGELOG.md)
- **Contributing** — [CONTRIBUTING.md](CONTRIBUTING.md)
- **Security** — [SECURITY.md](SECURITY.md)
- **License** — [Apache 2.0](LICENSE)
