"""Demo metric profiles, time-series synthesis, and log scenarios."""

import math
import random
from datetime import datetime, timedelta
from typing import Callable

METRIC_PROFILES = [
    # Proxmox nodes — moderate CPU, high memory
    ("asset-pve1",    "cpu_percent",    "%",    35,  8,  12, 0,    [(3, 2, 30)]),  # spike during container storm
    ("asset-pve1",    "memory_percent", "%",    72,  4,  5,  0,    []),
    ("asset-pve1",    "disk_percent",   "%",    52,  1,  0,  0.1,  []),
    ("asset-pve1",    "network_rx_mbps","Mbps", 15,  8,  10, 0,    [(3, 2, 40)]),
    ("asset-pve2",    "cpu_percent",    "%",    28,  6,  10, 0,    []),
    ("asset-pve2",    "memory_percent", "%",    58,  5,  4,  0,    []),
    ("asset-pve2",    "disk_percent",   "%",    44,  1,  0,  0.05, []),
    ("asset-pve2",    "network_rx_mbps","Mbps", 8,   5,  6,  0,    []),
    # TrueNAS — high disk trending up, moderate network
    ("asset-truenas", "cpu_percent",    "%",    25,  10, 8,  0,    [(0, 4, 15)]),  # scrub spike
    ("asset-truenas", "memory_percent", "%",    68,  3,  2,  0,    []),
    ("asset-truenas", "disk_percent",   "%",    84,  0.5,0,  1.05, []),  # trending up ~1%/day, hitting 91 now
    ("asset-truenas", "network_rx_mbps","Mbps", 45,  20, 30, 0,    [(0, 2, 80)]),  # backup bursts
    # PBS backup — periodic spikes during backup windows
    ("asset-pbs",     "cpu_percent",    "%",    10,  5,  3,  0,    []),
    ("asset-pbs",     "memory_percent", "%",    42,  4,  2,  0,    []),
    ("asset-pbs",     "disk_percent",   "%",    65,  0.5,0,  0.2,  []),
    ("asset-pbs",     "network_rx_mbps","Mbps", 5,   15, 20, 0,    []),
    # OPNsense — low everything, steady
    ("asset-opnsense","cpu_percent",    "%",    8,   3,  4,  0,    [(5, 0.5, 60)]),  # firmware update spike
    ("asset-opnsense","memory_percent", "%",    32,  2,  1,  0,    []),
    ("asset-opnsense","disk_percent",   "%",    18,  0.5,0,  0,    []),
    ("asset-opnsense","network_rx_mbps","Mbps", 120, 40, 60, 0,    []),
    # Pi-hole — very low resource, steady DNS traffic
    ("asset-pihole",  "cpu_percent",    "%",    6,   3,  3,  0,    []),
    ("asset-pihole",  "memory_percent", "%",    28,  3,  2,  0,    []),
    ("asset-pihole",  "disk_percent",   "%",    15,  0.3,0,  0,    []),
    ("asset-pihole",  "network_rx_mbps","Mbps", 0.8, 0.4,0.5,0,   []),
    # UniFi — low resources
    ("asset-unifi",   "cpu_percent",    "%",    12,  4,  5,  0,    []),
    ("asset-unifi",   "memory_percent", "%",    45,  3,  2,  0,    []),
    ("asset-unifi",   "disk_percent",   "%",    22,  0.3,0,  0,    []),
    ("asset-unifi",   "network_rx_mbps","Mbps", 2,   1,  1.5,0,   []),
    # Docker prod — moderate, spike during incident
    ("asset-docker",  "cpu_percent",    "%",    22,  8,  10, 0,    [(3, 1, 70)]),  # container storm
    ("asset-docker",  "memory_percent", "%",    62,  5,  4,  0,    [(3, 1, 25)]),
    ("asset-docker",  "disk_percent",   "%",    48,  1,  0,  0.1,  []),
    ("asset-docker",  "network_rx_mbps","Mbps", 8,   5,  6,  0,    [(3, 1, 50)]),
    # K3s master — moderate steady workload
    ("asset-k3s-m",   "cpu_percent",    "%",    30,  8,  8,  0,    []),
    ("asset-k3s-m",   "memory_percent", "%",    58,  4,  3,  0,    []),
    ("asset-k3s-m",   "disk_percent",   "%",    40,  0.5,0,  0.05, []),
    ("asset-k3s-m",   "network_rx_mbps","Mbps", 12,  6,  8,  0,    []),
    # K3s worker
    ("asset-k3s-w1",  "cpu_percent",    "%",    35,  10, 10, 0,    []),
    ("asset-k3s-w1",  "memory_percent", "%",    65,  5,  4,  0,    []),
    ("asset-k3s-w1",  "disk_percent",   "%",    38,  0.5,0,  0.05, []),
    ("asset-k3s-w1",  "network_rx_mbps","Mbps", 10,  5,  7,  0,    []),
    # Home Assistant — low resources
    ("asset-hass",    "cpu_percent",    "%",    14,  5,  6,  0,    []),
    ("asset-hass",    "memory_percent", "%",    48,  4,  3,  0,    []),
    ("asset-hass",    "disk_percent",   "%",    35,  0.3,0,  0.02, []),
    ("asset-hass",    "network_rx_mbps","Mbps", 1.5, 0.8,1,  0,    []),
    # Media stack — bursty CPU/network during transcodes
    ("asset-media",   "cpu_percent",    "%",    20,  15, 12, 0,    []),
    ("asset-media",   "memory_percent", "%",    78,  5,  4,  0,    []),
    ("asset-media",   "disk_percent",   "%",    55,  0.5,0,  0.1,  []),
    ("asset-media",   "network_rx_mbps","Mbps", 30,  40, 25, 0,    []),
    # Monitoring — moderate steady
    ("asset-mon",     "cpu_percent",    "%",    18,  5,  6,  0,    []),
    ("asset-mon",     "memory_percent", "%",    55,  4,  3,  0,    []),
    ("asset-mon",     "disk_percent",   "%",    32,  0.3,0,  0.05, []),
    ("asset-mon",     "network_rx_mbps","Mbps", 5,   3,  4,  0,    []),
    # GitLab runner — bursty during work hours
    ("asset-gitlab",  "cpu_percent",    "%",    15,  20, 25, 0,    []),
    ("asset-gitlab",  "memory_percent", "%",    40,  10, 8,  0,    []),
    ("asset-gitlab",  "disk_percent",   "%",    52,  1,  0,  0.1,  []),
    ("asset-gitlab",  "network_rx_mbps","Mbps", 3,   8,  10, 0,    []),
    # Dev workstation — offline for last 3h
    ("asset-dev-ws",  "cpu_percent",    "%",    55,  15, 15, 0,    []),
    ("asset-dev-ws",  "memory_percent", "%",    72,  6,  5,  0,    []),
    ("asset-dev-ws",  "disk_percent",   "%",    68,  0.5,0,  0,    []),
    ("asset-dev-ws",  "network_rx_mbps","Mbps", 5,   4,  6,  0,    []),
    # Minecraft — evening spikes when players online
    ("asset-mc",      "cpu_percent",    "%",    8,   5,  15, 0,    []),  # high amplitude = evening gaming
    ("asset-mc",      "memory_percent", "%",    62,  8,  10, 0,    []),
    ("asset-mc",      "disk_percent",   "%",    35,  0.3,0,  0.02, []),
    ("asset-mc",      "network_rx_mbps","Mbps", 1,   2,  5,  0,    []),
    # Offsite backup — low except during nightly sync
    ("asset-offsite", "cpu_percent",    "%",    5,   3,  2,  0,    []),
    ("asset-offsite", "memory_percent", "%",    30,  3,  2,  0,    []),
    ("asset-offsite", "disk_percent",   "%",    42,  0.3,0,  0.1,  []),
    ("asset-offsite", "network_rx_mbps","Mbps", 0.5, 5,  15, 0,    []),  # nightly sync spikes
    # Windows HTPC — low idle, evening media playback
    ("asset-htpc",    "cpu_percent",    "%",    5,   3,  10, 0,    []),
    ("asset-htpc",    "memory_percent", "%",    45,  5,  5,  0,    []),
    ("asset-htpc",    "disk_percent",   "%",    60,  0.3,0,  0.02, []),
    ("asset-htpc",    "network_rx_mbps","Mbps", 2,   5,  15, 0,    []),
]


def generate_metric_series(asset_id: str, metric: str, unit: str, base: float, variance: float,
                           daily_amp: float, trend_per_day: float, spikes: list,
                           interval_minutes: int = 15, days: int = 7, *, now: datetime) -> list:
    """Generate realistic time-series data with day/night cycles, trends, spikes, and noise."""
    samples = []
    # Dev workstation went offline 3h ago — stop generating data then
    cutoff = now
    if asset_id == "asset-dev-ws":
        cutoff = now - timedelta(hours=3)

    total_points = (days * 24 * 60) // interval_minutes
    for i in range(total_points):
        t = now - timedelta(days=days) + timedelta(minutes=i * interval_minutes)
        if t > cutoff:
            break

        # Day/night cycle (peak at 14:00, trough at 04:00)
        hour = t.hour + t.minute / 60.0
        cycle = daily_amp * math.sin((hour - 4) * math.pi / 12)

        # Linear trend
        days_elapsed = i * interval_minutes / (24 * 60)
        trend = trend_per_day * days_elapsed

        # Spike overlay
        spike_add = 0
        for spike_days_ago, spike_duration_h, spike_amount in spikes:
            spike_center = now - timedelta(days=spike_days_ago)
            spike_start = spike_center - timedelta(hours=spike_duration_h / 2)
            spike_end = spike_center + timedelta(hours=spike_duration_h / 2)
            if spike_start <= t <= spike_end:
                # Bell curve within spike window
                progress = (t - spike_start).total_seconds() / max((spike_end - spike_start).total_seconds(), 1)
                spike_add = spike_amount * math.exp(-8 * (progress - 0.5) ** 2)

        # Gaussian noise
        noise = random.gauss(0, variance)

        value = base + cycle + trend + spike_add + noise
        value = max(0, min(100 if "percent" in metric else 999, value))
        value = round(value, 1)
        samples.append((asset_id, metric, unit, value, t))

    return samples


# ════════════════════════════════════════════════════════════════
# 8. LOG EVENTS — 200+ spread across 7 days
# ════════════════════════════════════════════════════════════════

def generate_logs(now: datetime, new_id: Callable[[], str]) -> list:
    logs = []

    def log(asset_id, source, level, message, fields, t):
        logs.append((new_id(), asset_id, source, level, message, fields, t))

    # -- Ongoing / recent --
    log("asset-pve1", "agent", "info", "Heartbeat OK: cpu=35.2% mem=72.1% vms=8/8 running", None, now - timedelta(minutes=1))
    log("asset-pve1", "agent", "info", "VM docker-prod: status=running, uptime=14d 3h", None, now - timedelta(minutes=30))
    log("asset-pve1", "agent", "warning", "SMART warning on /dev/sdc: reallocated sector count=12 (increasing)", {"disk":"/dev/sdc","attribute":"Reallocated_Sector_Ct","value":12}, now - timedelta(hours=2))
    log("asset-pve1", "agent", "info", "ZFS pool rpool: healthy, 52% used, last scrub 3d ago", None, now - timedelta(hours=4))
    log("asset-pve2", "agent", "info", "Heartbeat OK: cpu=28.4% mem=58.3% vms=4/4 running", None, now - timedelta(minutes=1))
    log("asset-pve2", "agent", "info", "Live migration of mc-server completed in 12s", None, now - timedelta(days=2))

    # TrueNAS — storage alerts
    log("asset-truenas", "agent", "error",   "ZFS pool tank: usage 91.3% exceeds 90% threshold", {"pool":"tank","used_pct":91.3}, now - timedelta(minutes=5))
    log("asset-truenas", "agent", "warning", "Scrub completed with 0 errors, runtime 6h 14m", None, now - timedelta(hours=8))
    log("asset-truenas", "agent", "info",    "NFS exports: 6 active clients, 4.2 GB/s aggregate throughput", None, now - timedelta(minutes=10))
    log("asset-truenas", "agent", "warning", "Disk temperature /dev/da4: 49C (threshold 50C)", {"disk":"/dev/da4","temp_c":49}, now - timedelta(hours=3))
    log("asset-truenas", "agent", "info",    "Snapshot auto-created: tank/vms@auto-2026-04-16-06:00", None, now - timedelta(hours=6))
    log("asset-truenas", "agent", "warning", "Pool tank: 14-day snapshot retention consuming 6.1 TB", None, now - timedelta(hours=12))

    # PBS backup
    log("asset-pbs", "agent", "info",    "Backup job completed: 8 VMs, 18.6 GB total, duration 52m, dedup 3.2:1", {"vms":8,"size_gb":18.6,"duration_min":52,"dedup_ratio":3.2}, now - timedelta(hours=1))
    log("asset-pbs", "agent", "info",    "Verification passed: all 8 VM backups integrity OK", None, now - timedelta(minutes=50))
    log("asset-pbs", "agent", "info",    "Pruning old backups: removed 12 snapshots, freed 24.3 GB", None, now - timedelta(hours=6))
    log("asset-pbs", "agent", "info",    "Datastore usage: 65.8% (retention policy: 14 days, 3 keep-daily)", None, now - timedelta(hours=1))

    # OPNsense
    log("asset-opnsense", "agent", "info",    "Firewall rules: 142 active, 0 blocked in last hour from LAN", None, now - timedelta(minutes=15))
    log("asset-opnsense", "agent", "info",    "WireGuard tunnel wg0: 2 peers connected, 12.4 MB transferred", None, now - timedelta(minutes=30))
    log("asset-opnsense", "agent", "info",    "Suricata IDS: 0 alerts in last 24h, 2.1M packets inspected", None, now - timedelta(hours=1))

    # Pi-hole
    log("asset-pihole", "agent", "info",    "DNS queries last hour: 14,231 (16.8% blocked)", {"queries":14231,"blocked_pct":16.8}, now - timedelta(minutes=1))
    log("asset-pihole", "agent", "info",    "Gravity database updated: 152,891 domains on blocklist", None, now - timedelta(hours=12))
    log("asset-pihole", "agent", "warning", "High query rate from 10.0.1.31: 3,200 queries in 5min (media-stack)", {"client":"10.0.1.31","queries":3200}, now - timedelta(hours=2))

    # UniFi
    log("asset-unifi", "agent", "info",    "Access points: 12/12 online, 47 clients connected", {"aps_online":12,"clients":47}, now - timedelta(minutes=5))
    log("asset-unifi", "agent", "warning", "AP-Garage signal strength degraded: -78 dBm (was -65 dBm)", {"ap":"AP-Garage","signal_dbm":-78}, now - timedelta(hours=4))
    log("asset-unifi", "agent", "info",    "Switch USW-Pro-24 firmware updated to 7.1.26", None, now - timedelta(days=1))

    # Docker prod
    log("asset-docker", "agent", "info",    "Containers: 22 running, 3 stopped, 3 paused", {"running":22,"stopped":3,"paused":3}, now - timedelta(minutes=2))
    log("asset-docker", "agent", "info",    "Container grafana: healthy, uptime 14d", None, now - timedelta(minutes=10))
    log("asset-docker", "agent", "warning", "Container nginx-proxy: 2 restarts in last hour", {"container":"nginx-proxy","restarts":2}, now - timedelta(minutes=45))
    log("asset-docker", "agent", "info",    "Docker image prune: reclaimed 4.1 GB", None, now - timedelta(hours=6))

    # K3s
    log("asset-k3s-m", "agent", "info",    "Pods: 35 running, 1 pending, 1 crashloop in cert-manager", {"running":35,"pending":1,"crashloop":1}, now - timedelta(minutes=1))
    log("asset-k3s-m", "agent", "error",   "Pod cert-manager-webhook-7f9b4c8d6-x2k4m: CrashLoopBackOff (12 restarts)", {"pod":"cert-manager-webhook","restarts":12,"namespace":"cert-manager"}, now - timedelta(minutes=5))
    log("asset-k3s-m", "agent", "info",    "Deployment ingress-nginx: 3/3 replicas ready", None, now - timedelta(hours=1))
    log("asset-k3s-m", "agent", "info",    "Certificate renewed: *.lab.local (expires in 87 days)", None, now - timedelta(days=1))
    log("asset-k3s-w1", "agent", "info",   "Pods: 18 running, 0 pending, 0 failed", {"running":18}, now - timedelta(minutes=1))
    log("asset-k3s-w1", "agent", "warning","PVC pvc-prometheus-data: filesystem 89% full", {"pvc":"pvc-prometheus-data","used_pct":89}, now - timedelta(hours=2))

    # Home Assistant
    log("asset-hass", "agent", "info",    "Automation triggered: lights_off_away (geofence: all departed)", None, now - timedelta(hours=3))
    log("asset-hass", "agent", "info",    "Integration reload: mqtt (214 entities updated)", None, now - timedelta(hours=8))
    log("asset-hass", "agent", "warning", "Z-Wave device unresponsive: front_door_lock (node 14)", {"node_id":14,"device":"front_door_lock"}, now - timedelta(hours=1))
    log("asset-hass", "agent", "error",   "Integration error: hue_bridge connection refused (retry in 30s)", {"integration":"hue_bridge"}, now - timedelta(minutes=20))
    log("asset-hass", "agent", "info",    "Energy dashboard: solar production 4.2 kWh today, grid import 1.8 kWh", None, now - timedelta(hours=2))

    # Media stack
    log("asset-media", "agent", "info",    "Active streams: 2 (1 direct play, 1 transcode 4K→1080p)", {"direct_play":1,"transcode":1}, now - timedelta(minutes=5))
    log("asset-media", "agent", "info",    "Library scan completed: 142 new items indexed across movies/tv/music", None, now - timedelta(hours=6))
    log("asset-media", "agent", "warning", "Transcode buffer underrun on stream 2 (client: LG TV)", {"stream_id":2,"client":"LG TV"}, now - timedelta(minutes=30))
    log("asset-media", "agent", "info",    "Hardware transcode: Intel QuickSync active, GPU utilization 58%", None, now - timedelta(minutes=15))
    log("asset-media", "agent", "info",    "Sonarr: downloaded 3 episodes, Radarr: 1 movie added to library", None, now - timedelta(hours=4))

    # Monitoring
    log("asset-mon", "agent", "info",    "Prometheus: 892 active targets, 0 down, scrape duration p99=1.2s", {"targets":892,"down":0}, now - timedelta(minutes=2))
    log("asset-mon", "agent", "info",    "Grafana: 14 dashboards, 8 alert rules, 3 firing", None, now - timedelta(minutes=10))
    log("asset-mon", "agent", "info",    "Loki: ingestion rate 3.4 MB/s, 14-day retention, 48 GB stored", None, now - timedelta(minutes=15))
    log("asset-mon", "agent", "warning", "Alertmanager: notification queue depth 62 (threshold 100)", {"queue_depth":62}, now - timedelta(hours=1))

    # GitLab runner
    log("asset-gitlab", "agent", "info",    "CI runner: 12 pipelines completed today, 1 failed, avg duration 8m", {"pipelines":12,"failed":1,"avg_min":8}, now - timedelta(minutes=30))
    log("asset-gitlab", "agent", "warning", "Pipeline #1847 failed: test stage timeout after 30m", {"pipeline":1847,"stage":"test"}, now - timedelta(hours=4))
    log("asset-gitlab", "agent", "info",    "Runner cache hit rate: 78%, cache size: 2.8 GB", None, now - timedelta(hours=2))

    # Dev workstation (last logs before going offline)
    log("asset-dev-ws", "agent", "info",    "CI runner: 3 jobs completed, 0 failed", None, now - timedelta(hours=3, minutes=10))
    log("asset-dev-ws", "agent", "warning", "Disk I/O latency elevated: avg 52ms (threshold 20ms)", {"avg_ms":52,"threshold_ms":20}, now - timedelta(hours=3, minutes=5))
    log("asset-dev-ws", "agent", "error",   "Agent lost connection to hub, attempting reconnect", None, now - timedelta(hours=3, minutes=2))
    log("asset-dev-ws", "agent", "error",   "Heartbeat timeout: no response in 300s, marking offline", None, now - timedelta(hours=3))

    # Minecraft
    log("asset-mc", "agent", "info",    "Server: 0/20 players online, TPS 20.0, world size 4.2 GB", {"players":0,"tps":20.0}, now - timedelta(minutes=5))
    log("asset-mc", "agent", "info",    "Peak concurrent: 8 players at 20:45, TPS stayed above 19.2", {"peak":8}, now - timedelta(hours=10))
    log("asset-mc", "agent", "info",    "World backup completed: 4.2 GB → PBS in 45s", None, now - timedelta(hours=6))

    # Offsite backup
    log("asset-offsite", "agent", "info",    "Nightly sync completed: 42.3 GB transferred in 3h 12m via WireGuard", {"size_gb":42.3,"duration_min":192}, now - timedelta(hours=5))
    log("asset-offsite", "agent", "info",    "WireGuard tunnel: latency 28ms, bandwidth 30 Mbps sustained", None, now - timedelta(hours=4))
    log("asset-offsite", "agent", "info",    "Datastore verification: 847 snapshots, all checksums valid", None, now - timedelta(days=1))

    # Windows HTPC
    log("asset-htpc", "agent", "info",    "Windows Update: 0 pending, last checked 2h ago", None, now - timedelta(hours=2))
    log("asset-htpc", "agent", "info",    "Plex client: idle, last stream ended 3h ago", None, now - timedelta(hours=3))

    # Hub-level logs
    log(None, "hub", "info",    "Alert rule evaluated: rule-disk-full → firing for truenas-scale",               None, now - timedelta(minutes=5))
    log(None, "hub", "info",    "Incident inc-nas-storage updated: new timeline event added",                    None, now - timedelta(hours=1))
    log(None, "hub", "info",    "User admin logged in from 192.168.1.100",                                       None, now - timedelta(hours=4))
    log(None, "hub", "info",    "Agent checkin summary: 17/18 assets reporting healthy",                         None, now - timedelta(minutes=2))
    log(None, "hub", "info",    "Scheduled job completed: metric_cleanup (removed 42,103 samples older than 30d)", None, now - timedelta(hours=12))
    log(None, "hub", "warning", "Rate limit triggered for /api/v1/metrics (client 10.0.1.40, 120 req/min)",       None, now - timedelta(hours=6))
    log(None, "hub", "info",    "TLS certificate auto-renewed for hub.lab.local, expires in 87 days",            None, now - timedelta(days=1))
    log(None, "hub", "info",    "Database vacuum completed: 18 tables, freed 312 MB",                            None, now - timedelta(hours=18))
    log(None, "hub", "info",    "Demo mode: read-only session provisioned for visitor from 203.0.113.42",        None, now - timedelta(hours=2))

    # -- Incident-correlated log bursts --
    # Container storm (3 days ago) — dense burst
    storm_base = now - timedelta(days=3)
    for i in range(20):
        t = storm_base + timedelta(seconds=i * 15)
        container = random.choice(["nginx-proxy", "redis-cache", "postgres-app", "grafana", "loki", "traefik", "authelia", "vaultwarden"])
        msg = random.choice([
            f"Container {container}: pull failed (429 Too Many Requests)",
            f"Container {container}: restart attempt {random.randint(1,8)} of 10",
            f"Container {container}: OOMKilled, exit code 137",
            f"Container {container}: health check failed (timeout 30s)",
            f"Container {container}: image not found in local cache, pulling...",
        ])
        log("asset-docker", "agent", random.choice(["error", "error", "warning"]), msg, {"container": container}, t)

    # Network outage burst (5 days ago)
    outage_base = now - timedelta(days=5) + timedelta(minutes=5)
    for asset in ["asset-pve1", "asset-pve2", "asset-docker", "asset-k3s-m", "asset-media", "asset-mon", "asset-hass"]:
        log(asset, "agent", "error", "Connection to hub lost: network unreachable", None, outage_base + timedelta(seconds=random.randint(0, 30)))
        log(asset, "agent", "info", "Connection to hub restored", None, outage_base + timedelta(minutes=45, seconds=random.randint(0, 60)))

    return logs


# ════════════════════════════════════════════════════════════════
# 9. AUDIT EVENTS
# ════════════════════════════════════════════════════════════════
