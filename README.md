<div align="center">

# LabTether

### Stop tab-hopping. Start operating.

One control plane for your entire homelab.<br/>
Metrics. Logs. Alerts. Remote access. Actions. One URL.

<br/>

[![CI](https://img.shields.io/github/actions/workflow/status/labtether/labtether/ci.yml?style=flat-square&label=CI)](https://github.com/labtether/labtether/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)](https://docs.docker.com/compose/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square)](LICENSE)

[Website](https://labtether.com) · [Docs](https://labtether.com/docs) · [Wiki](https://labtether.com/docs/wiki) · [Changelog](CHANGELOG.md)

</div>

<br/>

<!-- TODO: Replace with hero screenshot once demo is ready -->
<!-- <p align="center"><img src="docs/images/dashboard-dark.png" width="900" /></p> -->

---

## 🧩 The Problem

You run Proxmox, TrueNAS, Docker, maybe Home Assistant. Each has its own dashboard, its own alerts, its own way of telling you something is wrong.

When a drive starts failing at 2am, you're opening **five tabs** trying to piece together what happened.

You've tried the Grafana + Prometheus + Loki stack. It works — if you enjoy writing PromQL queries and maintaining **yet another set of infrastructure** just to monitor your infrastructure.

## 🔧 The Fix

LabTether replaces the tab sprawl with **one dashboard, one timeline, one URL.**

<table>
<tr>
<td width="50%" valign="top">

### 📊 See Everything
Fleet health, telemetry, and logs from every node. CPU, memory, disk, network, temperature — no PromQL required.

### 🖥️ Fix Things Fast
Remote terminal and desktop sessions directly in the browser. No SSH keys, no VNC clients, no port forwarding.

### 🔔 Know What Changed
Every action is audit-logged. Every alert is correlated with metrics and logs. Triage from a single incident timeline.

</td>
<td width="50%" valign="top">

### 🏗️ Manage It All
Linux, Windows, macOS, FreeBSD. Proxmox, TrueNAS, Docker, Portainer, Home Assistant. One place.

### 🔒 Own Your Data
Self-hosted with Docker Compose + Postgres. Nothing phones home. Nothing leaves your network. Ever.

### 🔄 Ship Updates Safely
Plan maintenance with dry-run support, rollback awareness, and full audit trails across your fleet.

</td>
</tr>
</table>

---

## 🚀 Quick Start

> [!TIP]
> You need **Docker** and **Docker Compose**. That's it.

**1. Grab the Compose file**

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

Open **https://localhost:8443** — TLS certificates are generated on first boot. The setup wizard walks you through the rest.

> [!NOTE]
> Full guide with **Tailscale remote access**, custom TLS, OIDC SSO, and multi-user setup at [labtether.com/docs](https://labtether.com/docs)

---

## 📡 Add Your Nodes

Agents are optional — connectors like Proxmox and TrueNAS work without them. But agents unlock **remote access**, deeper telemetry, and action execution.

<details>
<summary>🐧 <strong>Linux</strong></summary>
<br/>

Download the agent binary from [labtether-linux/releases](https://github.com/labtether/labtether-linux/releases) (amd64 or arm64), then enroll:

```bash
chmod +x labtether-agent-linux-amd64
sudo ./labtether-agent-linux-amd64 --hub wss://your-hub:8443/ws/agent --enrollment-token YOUR_TOKEN
```

Or use the hub's generated install command — it handles the download automatically.

📖 [Full setup guide →](https://labtether.com/docs/install-upgrade/agent-install-commands-by-os)

</details>

<details>
<summary>🍎 <strong>macOS</strong></summary>
<br/>

Download **LabTether Agent.app** from [labtether-mac/releases](https://github.com/labtether/labtether-mac/releases). Drag to Applications and launch — the menu bar icon handles enrollment.

📖 [Full setup guide →](https://labtether.com/docs/install-upgrade/agent-install-commands-by-os)

</details>

<details>
<summary>🪟 <strong>Windows</strong></summary>
<br/>

Download **LabTether Agent** from [labtether-win/releases](https://github.com/labtether/labtether-win/releases) and run the installer. The system tray icon handles enrollment.

📖 [Full setup guide →](https://labtether.com/docs/install-upgrade/agent-install-commands-by-os)

</details>

<details>
<summary>😈 <strong>FreeBSD</strong></summary>
<br/>

FreeBSD nodes are managed agentlessly via connectors. No agent install required.

</details>

---

## 🔌 Integrations

<p align="center">
  <img src="https://img.shields.io/badge/Proxmox%20VE-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="Proxmox VE" />
  <img src="https://img.shields.io/badge/Proxmox%20Backup-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="PBS" />
  <img src="https://img.shields.io/badge/TrueNAS-0095D5?style=for-the-badge&logo=truenas&logoColor=white" alt="TrueNAS" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/Portainer-13BEF9?style=for-the-badge&logo=portainer&logoColor=white" alt="Portainer" />
  <img src="https://img.shields.io/badge/Home%20Assistant-41BDF5?style=for-the-badge&logo=homeassistant&logoColor=white" alt="Home Assistant" />
</p>

LabTether pulls inventory, health, and telemetry from your infrastructure — not the other way around. Connect what you already run.

---

## 🧰 Ecosystem

| | Platform | What it does |
|:---|:---------|:-------------|
| 🖥️ **[Hub](https://github.com/labtether/labtether)** | Docker | The control plane. You're looking at it. |
| 🐧 **[Linux Agent](https://github.com/labtether/labtether-linux)** | Linux | Telemetry, remote access, and actions. |
| 🍎 **[macOS Agent](https://github.com/labtether/labtether-mac)** | macOS 13+ | Menu bar app with status and notifications. |
| 🪟 **[Windows Agent](https://github.com/labtether/labtether-win)** | Windows 10+ | System tray app with enrollment and credential management. |
| ⌨️ **[CLI](https://github.com/labtether/labtether-cli)** | Cross-platform | Manage your hub from the terminal. |
| 📱 **iOS Companion** | iPhone / iPad | Mobile fleet monitoring, push notifications, Live Activities. |

---

<div align="center">

**[Documentation](https://labtether.com/docs)** · **[Contributing](CONTRIBUTING.md)** · **[Security](SECURITY.md)** · **[Changelog](CHANGELOG.md)** · **[License](LICENSE)**

Copyright 2026 LabTether · [Apache 2.0](LICENSE)

</div>
