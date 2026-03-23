# Monitoring Guide

This document covers the observability endpoints, Prometheus metrics, and alerting
configuration available in LabTether hub.

## Health & Status Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/healthz` | GET | No | Liveness probe. Returns 200 when the process is running. |
| `/readyz` | GET | No | Readiness probe. Returns 200 when the hub can serve traffic (DB connected, migrations applied). |
| `/version` | GET | No | Returns build version, commit SHA, and build timestamp. |
| `/metrics` | GET | No | Prometheus metrics scrape endpoint (OpenMetrics format). |

The Docker health check in `docker-compose.deploy.yml` uses `/healthz`:

```yaml
healthcheck:
  test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://localhost:8080/healthz"]
  interval: 10s
  timeout: 5s
  retries: 3
  start_period: 15s
```

## Prometheus Metrics

All metrics are exposed at `/metrics` and carry the `labtether_` prefix.
The hub collects metrics via bridge adapters that run on a background loop
(see `cmd/labtether/startup_metrics_export.go`).

### Agent Presence

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_agent_connected` | gauge | `asset_id`, `asset_name`, `platform` | 1 if the agent has an active WebSocket connection, 0 otherwise. |
| `labtether_agent_last_heartbeat_age_seconds` | gauge | `asset_id`, `asset_name`, `platform` | Seconds since the agent's last heartbeat was received. |

### Docker Container Stats

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_cpu_used_percent` | gauge | `asset_id`, `docker_host`, `docker_image`, `docker_stack` | Container CPU usage percentage. |
| `labtether_memory_used_percent` | gauge | `asset_id`, `docker_host`, `docker_image`, `docker_stack` | Container memory usage percentage. |
| `labtether_network_rx_bytes_per_sec` | gauge | `asset_id`, `docker_host`, `docker_image` | Network receive rate (bytes/s). |
| `labtether_network_tx_bytes_per_sec` | gauge | `asset_id`, `docker_host`, `docker_image` | Network transmit rate (bytes/s). |
| `labtether_block_read_bytes_per_sec` | gauge | `asset_id`, `docker_host`, `docker_image` | Block device read rate (bytes/s). |
| `labtether_block_write_bytes_per_sec` | gauge | `asset_id`, `docker_host`, `docker_image` | Block device write rate (bytes/s). |
| `labtether_pids` | gauge | `asset_id`, `docker_host`, `docker_image` | Number of running processes in the container. |

### Alert State

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_alerts_firing` | gauge | — | Total number of alert instances currently firing. |
| `labtether_alerts_rules` | gauge | — | Total number of active alert rules. |
| `labtether_alert_evaluation_duration_ms` | gauge | `rule_name` | Last evaluation duration per alert rule (milliseconds). |

### Site Reliability

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_site_reliability_score` | gauge | `site_id`, `site_name` | Reliability score for an asset group (0-100). |

### Asset Info

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_asset_info` | gauge (always 1) | `asset_id`, `asset_name`, `asset_type`, `platform`, `docker_host`, `docker_image`, `docker_stack` | Static metadata series for PromQL joins. |

## Configuring Agent Monitoring Alerts

### Prometheus Alerting Rules

Create a Prometheus alerting rule file (e.g., `labtether-alerts.yml`) and load it
in your `prometheus.yml` under `rule_files:`.

#### Agent Offline Detection

```yaml
groups:
  - name: labtether-agent-health
    rules:
      - alert: AgentDown
        expr: labtether_agent_connected == 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Agent {{ $labels.asset_id }} is disconnected"
          description: "Agent {{ $labels.asset_name }} ({{ $labels.platform }}) has been offline for more than 5 minutes."
```

#### Stale Heartbeat Detection

```yaml
      - alert: AgentHeartbeatStale
        expr: labtether_agent_last_heartbeat_age_seconds > 300
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Agent {{ $labels.asset_id }} heartbeat is stale"
          description: "No heartbeat from {{ $labels.asset_name }} for {{ $value | humanizeDuration }}."
```

#### High Alert Fire Rate

```yaml
      - alert: TooManyAlertsFiring
        expr: labtether_alerts_firing > 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "{{ $value }} alerts are currently firing"
```

### Scrape Configuration

Add the hub as a Prometheus scrape target:

```yaml
scrape_configs:
  - job_name: labtether-hub
    scrape_interval: 15s
    static_configs:
      - targets: ["labtether:8080"]
```

## Grafana Dashboards

Pre-built dashboard definitions are maintained in `deploy/grafana/`:

| File | Description |
|------|-------------|
| `labtether-homelab-overview.json` | Top-level overview: agent count, alert state, reliability scores, per-agent connection status, alert eval duration. |
| `labtether-container-fleet.json` | Docker container fleet: CPU, memory, network, and block I/O across all managed Docker hosts. |
| `labtether-pbs-backup-health.json` | Proxmox Backup Server health: storage usage, backup counts, backup age, GC pending bytes. |

Import these into Grafana via the UI or provisioning. They expect a Prometheus
data source named `default` pointing at the hub's `/metrics` endpoint.
