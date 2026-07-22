# Monitoring Guide

This document covers the observability endpoints, Prometheus metrics, and alerting
configuration available in LabTether hub.

## Health & Status Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/healthz` | GET | No | Liveness probe. Returns 200 when the process is running. |
| `/readyz` | GET | No | Readiness probe. Returns 200 when the hub can serve traffic (DB connected, migrations applied). |
| `/version` | GET | No | Returns build version, commit SHA, and build timestamp. |
| `/metrics` | GET | `metrics:read` bearer | Prometheus metrics scrape endpoint (OpenMetrics format). |

The production Compose health check uses the container-local `/healthz`
listener that matches `LABTETHER_TLS_MODE` (HTTPS for the default `auto` mode,
HTTP only when TLS is explicitly disabled):

```yaml
healthcheck:
  test: ["CMD-SHELL", "if [ \"$${LABTETHER_TLS_MODE:-auto}\" = disabled ]; then exec wget -q -O /dev/null http://localhost:8080/healthz; else exec wget --no-check-certificate -q -O /dev/null https://localhost:8443/healthz; fi"]
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

### Opt-in Process Metrics

Process collection is disabled by default. Enable
`prometheus.process_metrics_enabled` in **Settings → Prometheus Export** (or
`LABTETHER_PROCESS_METRICS_ENABLED=true`) and set
`prometheus.process_metrics_top_n` from 1 to 200. The effective setting is read
on every collection cycle, so changes do not require a hub restart. The hub
asks only a rotating, bounded batch of connected agents and the agent request
and hub result are both capped to the configured top-N CPU processes.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_process_cpu_percent` | gauge | `asset_id`, `process_name`, `process_pid` | Process CPU percentage. |
| `labtether_process_memory_percent` | gauge | `asset_id`, `process_name`, `process_pid` | Process memory percentage. |
| `labtether_process_memory_rss_bytes` | gauge | `asset_id`, `process_name`, `process_pid` | Process resident memory in bytes. |

Commands, users and full command lines are never telemetry labels.

### Network Interfaces and Disk Mounts

Per-interface byte rates are derived from two bounded cumulative counter
snapshots. The first snapshot establishes a baseline and does not fabricate a
zero rate; counter resets likewise wait for a new baseline. Packet gauges are
the current cumulative counters reported by the agent.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_interface_rx_bytes_per_sec` | gauge | `asset_id`, `interface` | Derived receive rate in bytes/s. |
| `labtether_interface_tx_bytes_per_sec` | gauge | `asset_id`, `interface` | Derived transmit rate in bytes/s. |
| `labtether_interface_rx_packets` | gauge | `asset_id`, `interface` | Cumulative received packet count. |
| `labtether_interface_tx_packets` | gauge | `asset_id`, `interface` | Cumulative transmitted packet count. |
| `labtether_disk_total_bytes` | gauge | `asset_id`, `mount_point` | Mounted filesystem total bytes. |
| `labtether_disk_used_bytes` | gauge | `asset_id`, `mount_point` | Mounted filesystem used bytes. |
| `labtether_disk_available_bytes` | gauge | `asset_id`, `mount_point` | Mounted filesystem available bytes. |
| `labtether_disk_used_percent` | gauge | `asset_id`, `mount_point` | Mounted filesystem usage percentage. |

### Service Health and Synthetic Checks

Agent-discovered service health uses the host asset identity, bounded service
name and stable service ID. Raw service URLs are deliberately not emitted; the
historical `service_url` descriptor label stays empty. The stable service ID is
carried in the established `target` descriptor label.

Standalone synthetic checks are hub-global gauges with `scope="hub-synthetic"`.
Only check ID, display name and type are labels. Check targets are deliberately
excluded because URLs can contain credentials or high-cardinality query data.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_service_response_ms` | gauge | `asset_id`, `service_name`, `target` | Latest known response time; omitted when unavailable. |
| `labtether_service_uptime_percent` | gauge | `asset_id`, `service_name`, `target` | Rolling 24-hour uptime; omitted until history exists. |
| `labtether_service_status` | gauge | `asset_id`, `service_name`, `target` | 1 for up and 0 for down. |
| `labtether_synthetic_latency_ms` | gauge | `scope`, `check_id`, `check_name`, `check_type` | Latest persisted check latency; omitted when unavailable. |
| `labtether_synthetic_status` | gauge | `scope`, `check_id`, `check_name`, `check_type` | 0 for fail, 1 for OK and 2 for timeout. |

### Proxmox and PBS Sources

Proxmox and PBS do not run duplicate Prometheus-only polling bridges. The
existing scheduled connector collectors are the production sources, so API
load and metric timestamps match the inventory pass that produced the data.
Proxmox publishes its truthful CPU, memory and disk utilization through the
normal collector heartbeat telemetry; `/cluster/resources` cumulative network
bytes are not mislabeled as per-second rates, and the API exposes no disk-I/O
rate there.

The PBS collector emits the following datastore metrics from the status,
snapshot and GC responses it already fetched. Optional metrics are omitted
when the corresponding PBS call failed instead of being replaced with zero.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_storage_total_bytes` | gauge | `asset_id`, `datastore` | Datastore total bytes. |
| `labtether_storage_used_bytes` | gauge | `asset_id`, `datastore` | Datastore used bytes. |
| `labtether_storage_available_bytes` | gauge | `asset_id`, `datastore` | Datastore available bytes. |
| `labtether_backup_count` | gauge | `asset_id`, `datastore` | Current snapshot count. |
| `labtether_backup_age_seconds` | gauge | `asset_id`, `datastore` | Age of the newest backup. |
| `labtether_gc_pending_bytes` | gauge | `asset_id`, `datastore` | Bytes pending garbage collection. |

All bridge writes have finite-value and UTF-8/label envelope validation, a
50,000-sample append ceiling and a five-second persistence timeout. Agent
resource sweeps use at most 64 connected assets per cycle, concurrency eight
and a three-second per-agent response timeout; larger fleets rotate across
subsequent cycles.

### Alert State

Hub-global gauges use a dedicated descriptor path and carry a `scope` label;
they do not use a synthetic `asset_id`. Per-rule series use `rule_id` as the
stable identity because display names are not required to be unique.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_alerts_firing` | gauge | `scope` | Total number of alert instances currently firing. |
| `labtether_alerts_rules` | gauge | `scope` | Total number of active alert rules. |
| `labtether_alert_evaluation_duration_ms` | gauge | `scope`, `rule_id`, `rule_name` | Last evaluation duration per alert rule (milliseconds). |

### Site Reliability

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `labtether_site_reliability_score` | gauge | `scope`, `site_id`, `site_name` | Reliability score for an asset group (0-100). |

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

### Prometheus remote_write

The hub can push persisted asset and hub metrics to a Prometheus-compatible
`remote_write` receiver. Configure it under **Settings → Prometheus Export** or
through the runtime settings API. Enabling requires an absolute HTTP(S) URL;
userinfo, query strings, fragments, control characters, and overlong URLs or
credentials are rejected. Plain HTTP remains subject to the explicit global
insecure-transport opt-in. The push interval is bounded from 10 seconds to one
hour, and password bytes are encrypted at rest and never returned by the API.

Configuration changes take effect without restarting the hub. The runtime owns
one worker, one request at a time, with a maximum 500-sample/4-MiB request and a
bounded timeout/backoff. Oversized local pages and receiver HTTP 413 responses
are retried with progressively smaller pages, while full accepted pages catch
up at a bounded faster cadence instead of waiting for the steady-state
interval. It pages the asset and hub telemetry tables by their independent
insertion IDs, not timestamps, so delayed or equal-time samples are not skipped
at a batch boundary. Both cursors advance only after a 2xx receiver response and
a successful durable cursor update. If that final update fails, the page is
retried with at-least-once semantics and may be duplicated rather than silently
lost.

The cursor survives hub restarts. Rotating only the password retains it;
changing the receiver URL or tenant username resets it by a SHA-256 destination
fingerprint and replays retained telemetry for the new receiver. Logs and
client-facing errors never include the configured endpoint or credential
values.

## Grafana Dashboards

Pre-built dashboard definitions are maintained in `deploy/grafana/`:

| File | Description |
|------|-------------|
| `labtether-homelab-overview.json` | Top-level overview: agent count, alert state, reliability scores, per-agent connection status, alert eval duration. |
| `labtether-container-fleet.json` | Docker container fleet: CPU, memory, network, and block I/O across all managed Docker hosts. |
| `labtether-pbs-backup-health.json` | Proxmox Backup Server health: storage usage, backup counts, backup age, GC pending bytes. |

Import these into Grafana via the UI or provisioning. They expect a Prometheus
data source named `default` pointing at the hub's `/metrics` endpoint.
# Prometheus authentication

The hub's `/metrics` endpoint requires authentication. Create a narrowly
scoped API key with `metrics:read` and configure Prometheus with an
`Authorization: Bearer <key>` header. The metrics endpoint intentionally has
no unauthenticated network mode; deployments that cannot attach a bearer
header should place an authenticating loopback proxy beside the hub.
