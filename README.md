<p align="center">
  <h1 align="center">LabTether</h1>
  <p align="center">
    <strong>Stop tab-hopping. Start operating.</strong>
    <br />
    The self-hosted control plane for homelabs that deserve better than a pile of browser tabs.
  </p>
</p>

<p align="center">
  <a href="https://github.com/labtether/labtether/actions/workflows/ci.yml"><img src="https://github.com/labtether/labtether/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white" alt="Docker" />
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License" /></a>
</p>

<p align="center">
  <a href="https://labtether.com">Website</a> &bull;
  <a href="https://labtether.com/docs">Documentation</a> &bull;
  <a href="https://labtether.com/docs/wiki">Wiki</a> &bull;
  <a href="CHANGELOG.md">Changelog</a>
</p>

<!-- TODO: Hero screenshot — dashboard-dark.png -->

---

## The Problem

You run Proxmox, TrueNAS, Docker, maybe Home Assistant. Each has its own dashboard, its own alerts, its own way of telling you something is wrong. When a drive starts failing at 2am, you're opening five tabs trying to correlate what happened, when, and why.

You've tried Grafana + Prometheus + Loki. It works — if you enjoy writing PromQL and maintaining yet another stack. For most homelab operators, that's trading one problem for another.

## The Fix

LabTether puts everything in one place. Metrics, logs, alerts, incidents, remote access, and actions — one dashboard, one timeline, one URL.

- **See everything** — Fleet health, telemetry, and logs from every node. No PromQL required.
- **Fix things fast** — Remote terminal and desktop sessions directly from the browser. No SSH keys to manage.
- **Know what changed** — Every action is logged. Every alert is correlated with metrics. Triage incidents from a single timeline.
- **Manage it all** — Linux, Windows, macOS, FreeBSD. Proxmox, TrueNAS, Docker, Portainer, Home Assistant. One control plane.
- **Own your data** — Self-hosted with Docker Compose + Postgres. Nothing phones home. Nothing leaves your network.

<!-- TODO: Feature screenshots grid — dashboard, remote access, alerts, topology -->

---

## Quick Start

Get LabTether running in under 5 minutes. You need Docker and Docker Compose.

**1. Download the Compose file**

```bash
curl -fsSL https://raw.githubusercontent.com/labtether/labtether/main/docker-compose.deploy.yml \
  -o docker-compose.yml
```

**2. Configure**

```bash
cat > .env.deploy << 'EOF'
LABTETHER_VERSION=latest
LABTETHER_HUB_IMAGE=ghcr.io/labtether/labtether/labtether:${LABTETHER_VERSION}
LABTETHER_WEB_IMAGE=ghcr.io/labtether/labtether/web-console:${LABTETHER_VERSION}
POSTGRES_IMAGE=postgres:16-alpine
GUACD_IMAGE=guacamole/guacd:1.6.0
EOF
```

**3. Launch**

```bash
docker compose up -d
```

Open **https://localhost:8443** — the hub generates TLS certificates on first boot and walks you through setup.

> Full guide with Tailscale remote access, custom TLS, and multi-user setup at **[labtether.com/docs](https://labtether.com/docs)**

---

## Add Your Nodes

Agents run on your machines and report back to the hub. They're optional — connectors like Proxmox and TrueNAS work without agents — but agents unlock remote access, deeper telemetry, and action execution.

<details>
<summary><strong>Linux</strong></summary>

```bash
curl -fsSL https://github.com/labtether/labtether-linux/releases/latest/download/labtether-agent-linux-amd64 \
  -o /usr/local/bin/labtether-agent && chmod +x /usr/local/bin/labtether-agent
```

Then enroll: `labtether-agent --hub wss://your-hub:8443/ws/agent --enrollment-token YOUR_TOKEN`

Full setup with systemd: [labtether.com/docs/wiki/agents/linux](https://labtether.com/docs/wiki/agents/linux)

</details>

<details>
<summary><strong>macOS</strong></summary>

Download **LabTether Agent.app** from [Releases](https://github.com/labtether/labtether-mac/releases/latest) — a menu bar app that handles enrollment, status, and notifications.

Guide: [labtether.com/docs/wiki/agents/macos](https://labtether.com/docs/wiki/agents/macos)

</details>

<details>
<summary><strong>Windows</strong></summary>

Download **LabTether Agent** from [Releases](https://github.com/labtether/labtether-win/releases/latest) — a system tray app with auto-updates and service management.

Guide: [labtether.com/docs/wiki/agents/windows](https://labtether.com/docs/wiki/agents/windows)

</details>

<details>
<summary><strong>FreeBSD</strong></summary>

FreeBSD nodes are managed agentlessly via connectors. No agent install required.

</details>

---

## What You Get

### Fleet Dashboard
CPU, memory, disk, network, and temperature across every node — at a glance. Drill into any machine for full telemetry history.

### Remote Access
Open a terminal or desktop session to any agent-connected machine directly from the browser. WebRTC-powered, no port forwarding or VPN required (works great with Tailscale).

### Alerts & Incidents
Define alert rules from templates or custom conditions. When things break, triage from a single timeline that correlates metrics, logs, and changes.

### Integrations
Connect what you already run. LabTether pulls inventory, health, and telemetry from your infrastructure — not the other way around.

### Update Runs
Plan and execute maintenance across your fleet. Dry-run support, rollback awareness, and full audit trails.

---

## Integrations

<p>
  <img src="https://img.shields.io/badge/Proxmox%20VE-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="Proxmox VE" />
  <img src="https://img.shields.io/badge/Proxmox%20Backup-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="PBS" />
  <img src="https://img.shields.io/badge/TrueNAS-0095D5?style=for-the-badge&logo=truenas&logoColor=white" alt="TrueNAS" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/Portainer-13BEF9?style=for-the-badge&logo=portainer&logoColor=white" alt="Portainer" />
  <img src="https://img.shields.io/badge/Home%20Assistant-41BDF5?style=for-the-badge&logo=homeassistant&logoColor=white" alt="Home Assistant" />
</p>

---

## Ecosystem

| | Platform | Description |
|---|----------|-------------|
| **[Hub](https://github.com/labtether/labtether)** | Docker | The control plane. You're looking at it. |
| **[Linux Agent](https://github.com/labtether/labtether-linux)** | Linux | Telemetry, remote access, and actions for Linux machines. |
| **[macOS Agent](https://github.com/labtether/labtether-mac)** | macOS 13+ | Menu bar app with status, settings, and notifications. |
| **[Windows Agent](https://github.com/labtether/labtether-win)** | Windows 10+ | System tray app with Hyper-V and Windows Update support. |
| **[CLI](https://github.com/labtether/labtether-cli)** | Cross-platform | Manage your hub from the command line. |
| **iOS Companion** | iPhone / iPad | Coming to [labtether.com](https://labtether.com). |

---

## Links

| | |
|---|---|
| **Documentation** | [labtether.com/docs](https://labtether.com/docs) |
| **Contributing** | [CONTRIBUTING.md](CONTRIBUTING.md) |
| **Security** | [SECURITY.md](SECURITY.md) |
| **Changelog** | [CHANGELOG.md](CHANGELOG.md) |
| **License** | [Apache 2.0](LICENSE) |
