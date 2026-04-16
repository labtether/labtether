<div align="center">

<img src=".github/logo.svg" alt="LabTether" width="120" />

# LabTether

**Cross-platform homelab control plane with AI-powered operations.**

[![CI](https://github.com/labtether/labtether/actions/workflows/ci.yml/badge.svg)](https://github.com/labtether/labtether/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)](https://docs.docker.com/compose/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square)](LICENSE)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?style=flat-square&logo=discord&logoColor=white)](https://discord.gg/labtether)
[![Demo](https://img.shields.io/badge/Demo-Try%20It-FF0080?style=flat-square)](https://demo.labtether.com)

[Website](https://labtether.com) &middot; [Docs](https://labtether.com/docs) &middot; [Wiki](https://labtether.com/docs/wiki) &middot; [Discord](https://discord.gg/labtether) &middot; [Demo](https://demo.labtether.com) &middot; [Changelog](CHANGELOG.md)

</div>

<p align="center">
  <img src="screenshots/01-dashboard.png" alt="LabTether Dashboard" width="900" />
</p>

<p align="center">
  <img src="screenshots/walkthrough.gif" alt="LabTether Walkthrough" width="900" />
  <br/>
  <em>Dashboard &rarr; Devices &rarr; Topology &rarr; Terminal &rarr; Alerts &rarr; Files</em>
</p>

<table>
<tr>
<td align="center" width="33%">
<strong>AI-Native</strong><br/>
Built-in MCP server with 23+ tools. Operate your fleet through Claude, OpenClaw, Cursor, or any AI agent.
</td>
<td align="center" width="33%">
<strong>Cross-Platform</strong><br/>
Linux, Windows, macOS, FreeBSD — all first-class citizens, managed from one hub.
</td>
<td align="center" width="33%">
<strong>Secure by Default</strong><br/>
Tailscale integration, 2FA, OIDC/SSO, RBAC, audit logs, and TLS — out of the box.
</td>
</tr>
</table>

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

Get a full LabTether hub running in under 5 minutes. You need Docker.

**One command:**

```bash
docker run -d --name labtether \
  -p 3000:3000 -p 8443:8443 \
  -v labtether-data:/data \
  ghcr.io/labtether/labtether:latest
```

Open **http://localhost:3000** — the setup wizard walks you through the rest. TLS certificates are generated automatically.

> Full guide with Tailscale remote access, custom TLS, OIDC SSO, and multi-user setup at [labtether.com/docs](https://labtether.com/docs).

### Advanced: Docker Compose

For split services, external Postgres, or resource limits:

```bash
curl -fsSL https://raw.githubusercontent.com/labtether/labtether/main/deploy/compose/docker-compose.deploy.yml \
  -o docker-compose.yml
docker compose up -d
```

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

### AI-Powered Operations

<p align="center">
  <img src="https://img.shields.io/badge/MCP-Model%20Context%20Protocol-8A2BE2?style=for-the-badge" alt="MCP" />
  <img src="https://img.shields.io/badge/Claude-Compatible-F97316?style=for-the-badge&logo=anthropic&logoColor=white" alt="Claude" />
  <img src="https://img.shields.io/badge/OpenClaw-Compatible-00C853?style=for-the-badge" alt="OpenClaw" />
</p>

LabTether ships a built-in **MCP server** with 23+ tools. Connect Claude, OpenClaw, Cursor, or any MCP-compatible AI agent and operate your infrastructure through natural language:

- Run commands across your fleet, restart services, read logs
- Query container stats, manage Docker stacks, check connector health
- Browse files, acknowledge alerts, reboot or wake machines
- Explore topology, list schedules, trigger webhooks

> *"Restart the nginx container on prod-web-01"* -- your AI agent does it through LabTether's MCP endpoint.

### Secure Remote Access with Tailscale

One-click **Tailscale Serve** integration. Expose your hub over HTTPS to your tailnet -- no port forwarding, no dynamic DNS, no certificates to manage. Managed mode handles everything automatically.

### Fleet Dashboard

Health at a glance. CPU, memory, disk, network, and temperature across every node.

<p align="center">
  <img src="screenshots/01-dashboard.png" alt="Fleet Dashboard" width="800" />
</p>

### Remote Terminal and SSH

Browser-based shell sessions with snippets, bookmarks, and persistent sessions. Manage SSH connections, keys, and protocols per device. Quick-connect from the command palette -- no SSH keys needed.

<p align="center">
  <img src="screenshots/05-terminal.png" alt="Remote Terminal" width="800" />
</p>

### Remote Desktop

VNC and SPICE desktop access directly from the browser. Session recordings for audit and compliance.

### File Manager

Browse, upload, download, and transfer files between devices from one interface.

<p align="center">
  <img src="screenshots/06-files.png" alt="File Manager" width="800" />
</p>

### Alerts, Incidents, and Logs

Define alert rules, route notifications, silence during maintenance, and triage from one correlated timeline. Full incident tracking with postmortem analysis. Centralized log viewer with search, filtering, and journal access across all nodes.

<p align="center">
  <img src="screenshots/08-alerts.png" alt="Alerts" width="800" />
</p>

### Containers and Services

Monitor and manage Docker containers and Compose stacks across your fleet. Discover, group, and monitor web services with health checks and custom icons.

### Topology Map

Visualize your infrastructure, dependencies, and network relationships. Blast radius analysis for change planning.

<p align="center">
  <img src="screenshots/03-topology.png" alt="Topology Map" width="800" />
</p>

### Automation

Save and execute actions across devices. Webhooks and scheduled jobs for hands-free operations. Cron-like scheduling with group-aware execution.

### Integrations

Connect what you already run: Proxmox VE, TrueNAS, Docker, Portainer, Home Assistant, and Proxmox Backup Server. Auto-discovery finds your infrastructure automatically.

### Enterprise Security

2FA, OIDC/SSO, RBAC, API keys, TLS management, Prometheus metrics export, data retention policies, and a full audit log. Everything you need for compliance.

### And More

**Groups and Maintenance** -- Organize assets with maintenance windows and reliability tracking. **Update Runs** -- Plan and execute updates with dry-run support and audit trails. **Command Palette** -- Cmd+K to jump anywhere, quick-connect to devices, or run saved actions. **API Docs** -- Built-in OpenAPI reference for every endpoint.

<details>
<summary><strong>All Screenshots</strong></summary>
<br/>
<p align="center">
  <img src="screenshots/01-dashboard.png" alt="Fleet Dashboard" width="800" /><br/>
  <em>Fleet Dashboard — all nodes at a glance</em>
</p>
<p align="center">
  <img src="screenshots/02-devices.png" alt="Device Detail" width="800" /><br/>
  <em>Device Detail — deep telemetry per host</em>
</p>
<p align="center">
  <img src="screenshots/05-terminal.png" alt="Remote Terminal" width="800" /><br/>
  <em>Remote Terminal — browser-based shell with snippets and bookmarks</em>
</p>
<p align="center">
  <img src="screenshots/06-files.png" alt="File Manager" width="800" /><br/>
  <em>File Manager — browse, upload, download, and transfer files</em>
</p>
<p align="center">
  <img src="screenshots/04-services.png" alt="Services" width="800" /><br/>
  <em>Services — discover and monitor web services with health checks</em>
</p>
<p align="center">
  <img src="screenshots/03-topology.png" alt="Topology Map" width="800" /><br/>
  <em>Topology Map — visualize your infrastructure and dependencies</em>
</p>
<p align="center">
  <img src="screenshots/08-alerts.png" alt="Alerts and Incidents" width="800" /><br/>
  <em>Alerts — correlated timeline with routing and silencing</em>
</p>
<p align="center">
  <img src="screenshots/07-logs.png" alt="Logs" width="800" /><br/>
  <em>Logs — centralized viewer with search and filtering</em>
</p>
<p align="center">
  <img src="screenshots/10-health.png" alt="Health Overview" width="800" /><br/>
  <em>Health — reliability tracking and system health checks</em>
</p>
<p align="center">
  <img src="screenshots/09-settings.png" alt="Settings" width="800" /><br/>
  <em>Settings — integrations, security, agents, and notifications</em>
</p>
<p align="center">
  <img src="screenshots/wave2-login.png" alt="Login" width="800" /><br/>
  <em>Login — 2FA, OIDC/SSO, and secure authentication</em>
</p>
</details>

---

## Supported Integrations

<p align="center">
  <img src="https://img.shields.io/badge/Proxmox%20VE-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="Proxmox VE" />
  <img src="https://img.shields.io/badge/Proxmox%20Backup-E57000?style=for-the-badge&logo=proxmox&logoColor=white" alt="Proxmox Backup Server" />
  <img src="https://img.shields.io/badge/TrueNAS-0095D5?style=for-the-badge&logo=truenas&logoColor=white" alt="TrueNAS" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/Portainer-13BEF9?style=for-the-badge&logo=portainer&logoColor=white" alt="Portainer" />
  <img src="https://img.shields.io/badge/Home%20Assistant-41BDF5?style=for-the-badge&logo=homeassistant&logoColor=white" alt="Home Assistant" />
  <img src="https://img.shields.io/badge/Tailscale-242424?style=for-the-badge&logo=tailscale&logoColor=white" alt="Tailscale" />
  <img src="https://img.shields.io/badge/MCP-8A2BE2?style=for-the-badge" alt="MCP" />
</p>

---

## Ecosystem

| | Platform | Description |
|:---|:---------|:------------|
| **[Linux Agent](https://github.com/labtether/labtether-agent)** | Linux | Telemetry, remote access, and actions for Linux machines. |
| **[CLI](https://github.com/labtether/labtether-cli)** | Cross-platform | Manage your hub from the terminal. |

### Coming soon

| | Platform | Description |
|:---|:---------|:------------|
| **[Windows Agent](https://github.com/labtether/labtether-win)** | Windows 10+ | Native system tray app with service management and auto-updates. |
| **[macOS Agent](https://github.com/labtether/labtether-mac)** | macOS 13+ | Menu bar app with status, enrollment, and notifications. |
| **FreeBSD Agent** | FreeBSD 13+ | Endpoint agent for BSD-based systems. |
| **iOS & iPad Companion** | iPhone / iPad | Mobile fleet monitoring, push notifications, and live activities. One-time purchase, no subscriptions. |

---

## Community

- **Discord** -- [Join the server](https://discord.gg/labtether)
- **Twitter/X** -- [@labtether](https://x.com/labtether) &middot; [@Watari_Labs_](https://x.com/Watari_Labs_)
- **Blog** -- [labtether.com/blog](https://labtether.com/blog)
- **Live Demo** -- [demo.labtether.com](https://demo.labtether.com) (no signup required)

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
