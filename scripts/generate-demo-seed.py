#!/usr/bin/env python3
"""Generate realistic demo seed SQL for LabTether hub.

Usage:
    python3 scripts/generate-demo-seed.py > scripts/demo-seed.sql

No external dependencies — stdlib only.
"""

import json
import random
import sys
import uuid
from datetime import datetime, timedelta, timezone

from demo_telemetry import METRIC_PROFILES, generate_metric_series, generate_logs

# ── Deterministic seed for reproducible output ─────────────────
random.seed(42)
NOW = datetime.now(timezone.utc).replace(second=0, microsecond=0)

def uid() -> str:
    return str(uuid.uuid4())

def ts(dt: datetime) -> str:
    return dt.strftime("%Y-%m-%d %H:%M:%S+00")

def sql_str(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"

def sql_json(obj) -> str:
    return sql_str(json.dumps(obj, separators=(",", ":"))) + "::jsonb"

def sql_null_or_str(s: str | None) -> str:
    return "NULL" if s is None else sql_str(s)

def sql_null_or_ts(dt: datetime | None) -> str:
    return "NULL" if dt is None else sql_str(ts(dt))


# ════════════════════════════════════════════════════════════════
# 1. GROUPS
# ════════════════════════════════════════════════════════════════

GROUPS = [
    ("grp-prod",       "Production",   "production",  None,          "server",   1),
    ("grp-compute",    "Compute",      "compute",     "grp-prod",    "cpu",      1),
    ("grp-storage",    "Storage",      "storage",     "grp-prod",    "database", 2),
    ("grp-network",    "Network",      "network",     "grp-prod",    "globe",    3),
    ("grp-services",   "Services",     "services",    "grp-prod",    "layers",   4),
    ("grp-dev",        "Development",  "development", None,          "code",     2),
    ("grp-remote",     "Remote",       "remote",      None,          "radio",    3),
]

# ════════════════════════════════════════════════════════════════
# 2. ASSETS — 18 power-user homelab nodes
# ════════════════════════════════════════════════════════════════

# (id, type, name, source, status, platform, host, transport, group_id, tags, metadata, created_days_ago, last_seen_offset)
ASSETS = [
    ("asset-pve1",       "server",    "pve-node-1",       "agent", "online",   "linux",   "10.0.1.1",  "agent", "grp-compute",  ["hypervisor","proxmox","cluster"],  {"os":"pve-8.3","arch":"amd64","agent_version":"1.4.2","cpu_cores":32,"ram_gb":128,"vms_running":8},    300, 30),
    ("asset-pve2",       "server",    "pve-node-2",       "agent", "online",   "linux",   "10.0.1.2",  "agent", "grp-compute",  ["hypervisor","proxmox","cluster"],  {"os":"pve-8.3","arch":"amd64","agent_version":"1.4.2","cpu_cores":16,"ram_gb":64,"vms_running":4},     300, 45),
    ("asset-truenas",    "nas",       "truenas-scale",    "agent", "degraded", "linux",   "10.0.1.3",  "agent", "grp-storage",  ["storage","zfs","nfs","iscsi"],     {"os":"truenas-scale-24.10","arch":"amd64","agent_version":"1.4.0","pool_size_tb":96,"drives":6,"raid":"raidz2"}, 300, 300),
    ("asset-pbs",        "server",    "pbs-backup",       "agent", "online",   "linux",   "10.0.1.4",  "agent", "grp-storage",  ["backup","pbs","dedup"],            {"os":"pbs-3.3","arch":"amd64","agent_version":"1.4.2","datastore_size_tb":12},                          200, 120),
    ("asset-opnsense",   "appliance", "opnsense",         "agent", "online",   "freebsd", "10.0.1.254","agent", "grp-network",  ["firewall","router","vpn"],         {"os":"opnsense-24.7","arch":"amd64","agent_version":"1.4.2","interfaces":6,"rules_active":142},          365, 20),
    ("asset-pihole",     "appliance", "pihole-primary",   "agent", "online",   "linux",   "10.0.1.5",  "agent", "grp-network",  ["dns","adblock","dhcp"],            {"os":"raspbian-12","arch":"arm64","agent_version":"1.4.2","domains_blocked":152891},                     365, 30),
    ("asset-unifi",      "appliance", "unifi-controller",  "agent", "online",   "linux",   "10.0.1.6",  "agent", "grp-network",  ["wifi","switching","unifi"],        {"os":"debian-12","arch":"amd64","agent_version":"1.4.2","aps":12,"switches":3,"clients":47},              180, 60),
    ("asset-docker",     "server",    "docker-prod",      "agent", "online",   "linux",   "10.0.1.10", "agent", "grp-services", ["docker","containers","compose"],   {"os":"debian-12","arch":"amd64","agent_version":"1.4.2","containers_running":22,"containers_total":28},  240, 40),
    ("asset-k3s-m",      "server",    "k3s-master",       "agent", "online",   "linux",   "10.0.1.20", "agent", "grp-services", ["kubernetes","k3s","control-plane"],{"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.4.2","pods_running":35,"namespaces":8},            120, 25),
    ("asset-k3s-w1",     "server",    "k3s-worker-1",     "agent", "online",   "linux",   "10.0.1.21", "agent", "grp-services", ["kubernetes","k3s","worker"],       {"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.4.2","pods_running":18},                           120, 30),
    ("asset-hass",       "appliance", "home-assistant",   "agent", "online",   "linux",   "10.0.1.30", "agent", "grp-services", ["automation","iot","zigbee"],       {"os":"haos-14.1","arch":"amd64","agent_version":"1.4.2","integrations":85,"entities":214,"automations":32}, 400, 45),
    ("asset-media",      "server",    "media-stack",      "agent", "online",   "linux",   "10.0.1.31", "agent", "grp-services", ["media","plex","sonarr","radarr"],  {"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.4.1","streams_active":2,"library_items":4280},      180, 90),
    ("asset-mon",        "server",    "monitoring",       "agent", "online",   "linux",   "10.0.1.40", "agent", "grp-services", ["observability","grafana","prometheus","loki"], {"os":"debian-12","arch":"amd64","agent_version":"1.4.2","targets":892,"dashboards":14}, 240, 50),
    ("asset-gitlab",     "vm",        "gitlab-runner",    "agent", "online",   "linux",   "10.0.1.50", "agent", "grp-dev",      ["ci","gitlab","runner"],            {"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.4.2","pipelines_today":12,"concurrent_jobs":4},     90, 60),
    ("asset-dev-ws",     "server",    "dev-workstation",  "agent", "offline",  "linux",   "10.0.1.51", "agent", "grp-dev",      ["development","workstation"],       {"os":"fedora-41","arch":"amd64","agent_version":"1.4.2"},                                                60, 10800),
    ("asset-mc",         "vm",        "minecraft-server", "agent", "online",   "linux",   "10.0.1.60", "agent", "grp-services", ["gaming","minecraft","java"],       {"os":"debian-12","arch":"amd64","agent_version":"1.4.2","players_peak":8,"world_size_gb":4.2},            45, 120),
    ("asset-offsite",    "server",    "offsite-backup",   "agent", "online",   "linux",   "10.10.1.1", "agent", "grp-remote",   ["backup","wireguard","offsite"],    {"os":"debian-12","arch":"amd64","agent_version":"1.4.2","vpn":"wireguard","last_sync":"2h ago"},          150, 180),
    ("asset-htpc",       "server",    "windows-htpc",     "agent", "online",   "windows", "10.0.1.70", "agent", "grp-services", ["media","htpc","windows"],          {"os":"windows-11-24H2","arch":"amd64","agent_version":"1.4.2"},                                          90, 300),
]


# ════════════════════════════════════════════════════════════════
# 3. TOPOLOGY EDGES — 25+
# ════════════════════════════════════════════════════════════════

# (source, target, rel_type, direction, criticality, origin, note)
EDGES = [
    # Storage → Compute
    ("asset-truenas", "asset-pve1",    "provides_to", "downstream", "critical", "manual", "NFS datastore for VMs"),
    ("asset-truenas", "asset-pve2",    "provides_to", "downstream", "critical", "manual", "NFS datastore for VMs"),
    # Compute → Services
    ("asset-pve1", "asset-docker",     "runs_on",     "downstream", "high",     "manual", "Docker host VM on Proxmox"),
    ("asset-pve1", "asset-k3s-m",      "runs_on",     "downstream", "high",     "manual", "K3s master VM"),
    ("asset-pve2", "asset-k3s-w1",     "runs_on",     "downstream", "high",     "manual", "K3s worker VM on node 2"),
    # Compute → Dev
    ("asset-pve1", "asset-gitlab",     "runs_on",     "downstream", "high",     "manual", "VM hosted on pve-node-1"),
    ("asset-pve1", "asset-dev-ws",     "runs_on",     "downstream", "medium",   "manual", "Dev workstation VM"),
    # K3s cluster
    ("asset-k3s-m",  "asset-k3s-w1",  "contains",    "downstream", "high",     "manual", "K3s cluster membership"),
    ("asset-k3s-m",  "asset-mon",      "runs_on",     "downstream", "high",     "manual", "Monitoring stack on K3s"),
    # Backup chain
    ("asset-pbs",     "asset-offsite", "depends_on",  "downstream", "high",     "manual", "Nightly offsite sync from PBS"),
    ("asset-truenas", "asset-pbs",     "provides_to", "downstream", "high",     "manual", "Backup target NFS share"),
]


# ════════════════════════════════════════════════════════════════
# 4. ALERT RULES
# ════════════════════════════════════════════════════════════════

ALERT_RULES = [
    ("rule-high-cpu",     "High CPU Usage",          "CPU usage above 80% for 5 minutes",                        "active", "metric_threshold","medium",   "global", 300, 60, 30, 300, {"metric":"cpu_percent","operator":">","threshold":80},           {"category":"performance"}),
    ("rule-disk-full",    "Disk Nearly Full",         "Disk usage above 90%",                                     "active", "metric_threshold","critical", "global", 300, 60, 30, 60,  {"metric":"disk_percent","operator":">","threshold":90},          {"category":"capacity"}),
    ("rule-mem-pressure", "Memory Pressure",          "Memory usage above 85% for 10 minutes",                    "active", "metric_threshold","high",     "global", 600, 60, 30, 600, {"metric":"memory_percent","operator":">","threshold":85},        {"category":"performance"}),
    ("rule-offline",      "Node Offline",             "Node has not reported in 5 minutes",                       "active", "heartbeat_stale", "critical", "global", 300, 60, 60, 300, {"stale_seconds":300},                                            {"category":"availability"}),
    ("rule-svc-down",     "Service Down",             "Critical service process not detected",                    "active", "log_pattern",     "critical", "global", 120, 30, 30, 120, {"pattern":"service (stopped|crashed|exited)","level":"error"},   {"category":"availability"}),
    ("rule-cert-expiry",  "Certificate Expiring",     "TLS certificate expires within 14 days",                   "active", "metric_threshold","high",     "global", 3600,60, 3600,86400,{"metric":"cert_days_remaining","operator":"<","threshold":14}, {"category":"security"}),
    ("rule-backup-stale", "Backup Stale",             "No successful backup in 36 hours",                         "active", "metric_deadman",  "high",     "global", 3600,60, 3600,129600,{"metric":"backup_last_success_hours","threshold":36},         {"category":"data-protection"}),
    ("rule-net-latency",  "Network Latency High",     "Inter-node latency above 50ms for 3 minutes",              "active", "metric_threshold","medium",   "global", 300, 60, 30, 180, {"metric":"net_latency_ms","operator":">","threshold":50},        {"category":"network"}),
]


# ════════════════════════════════════════════════════════════════
# 5. ALERT INSTANCES
# ════════════════════════════════════════════════════════════════

ALERT_INSTANCES = [
    # Firing: truenas disk at 91%
    ("ai-truenas-disk", "rule-disk-full",  "truenas-scale:disk_percent", "firing",       "critical",
     {"asset_id":"asset-truenas","metric":"disk_percent"},
     {"summary":"Disk usage at 91.3% on truenas-scale pool 'tank'","value":"91.3"},
     NOW - timedelta(hours=18), None, NOW - timedelta(minutes=5)),
    # Firing: k3s cert-manager crashloop
    ("ai-k3s-crash",   "rule-svc-down",   "k3s-master:cert-manager",    "firing",       "critical",
     {"asset_id":"asset-k3s-m","service":"cert-manager-webhook"},
     {"summary":"Pod cert-manager-webhook in CrashLoopBackOff (12 restarts)","value":"CrashLoopBackOff"},
     NOW - timedelta(hours=2), None, NOW - timedelta(minutes=3)),
    # Firing: dev workstation offline
    ("ai-dev-offline",  "rule-offline",    "dev-workstation:heartbeat",  "firing",       "high",
     {"asset_id":"asset-dev-ws"},
     {"summary":"dev-workstation offline for 3+ hours","value":"offline"},
     NOW - timedelta(hours=3), None, NOW - timedelta(minutes=1)),
    # Acknowledged: docker CPU spike
    ("ai-docker-cpu",   "rule-high-cpu",   "docker-prod:cpu_percent",    "acknowledged", "medium",
     {"asset_id":"asset-docker","metric":"cpu_percent"},
     {"summary":"CPU spike on docker-prod during container rebuild","value":"87.2"},
     NOW - timedelta(days=1), None, NOW - timedelta(hours=6)),
    # Pending: media memory approaching threshold
    ("ai-media-mem",    "rule-mem-pressure","media-stack:memory",        "pending",      "high",
     {"asset_id":"asset-media","metric":"memory_percent"},
     {"summary":"Memory approaching threshold on media-stack during transcode","value":"84.1"},
     NOW - timedelta(minutes=20), None, NOW - timedelta(minutes=3)),
    # Resolved: opnsense latency during firmware update
    ("ai-opn-latency",  "rule-net-latency","opnsense:net_latency",      "resolved",     "medium",
     {"asset_id":"asset-opnsense","metric":"net_latency_ms"},
     {"summary":"Network latency spike during OPNsense firmware update","value":"120"},
     NOW - timedelta(days=5), NOW - timedelta(days=5) + timedelta(minutes=45), NOW - timedelta(days=5, hours=-1)),
    # Resolved: backup stale (was during network outage)
    ("ai-backup-stale", "rule-backup-stale","pbs-backup:backup_age",    "resolved",     "high",
     {"asset_id":"asset-pbs"},
     {"summary":"PBS backup job stale during network outage window","value":"38h"},
     NOW - timedelta(days=5), NOW - timedelta(days=4, hours=12), NOW - timedelta(days=5)),
]


# ════════════════════════════════════════════════════════════════
# 6. INCIDENTS with rich timelines
# ════════════════════════════════════════════════════════════════

INCIDENTS = [
    # 1. NAS Storage Critical (open)
    {
        "id": "inc-nas-storage", "title": "NAS Storage Pool Critical — 91% Capacity",
        "summary": "ZFS pool 'tank' on truenas-scale has been trending upward for 7 days, crossing 90% threshold 18 hours ago. Current usage 91.3%. Snapshot retention reduced from 30d to 14d, reclaimed 2.1TB. Monitoring for stabilization before ordering expansion drives.",
        "status": "open", "severity": "critical", "source": "alert_auto",
        "group_id": "grp-storage", "primary_asset_id": "asset-truenas",
        "assignee": "admin", "created_by": "system",
        "opened_at": NOW - timedelta(hours=18),
        "events": [
            ("metric_anomaly", "truenas:disk:trend",     "Disk usage trend: 87.1% → 91.3% over 7 days, accelerating", "high",    NOW - timedelta(hours=18)),
            ("alert_fired",    "ai-truenas-disk",         "Alert fired: Disk Nearly Full on truenas-scale (91.3%)",     "critical",NOW - timedelta(hours=18)),
            ("audit",          "admin:triage:storage",     "Investigating ZFS pool. Running scrub to verify no errors.", "info",    NOW - timedelta(hours=16)),
            ("audit",          "admin:analysis:snapshots", "Identified snapshot retention as primary consumer: 8.2TB in 30-day snapshots across 4 datasets.", "info", NOW - timedelta(hours=14)),
            ("config_change",  "admin:config:retention",   "Reduced snapshot retention from 30d to 14d on tank/vms and tank/backups.", "info", NOW - timedelta(hours=12)),
            ("audit",          "admin:verify:reclaim",     "Retention change reclaimed 2.1TB. Pool now at 88.7% but new data still incoming.", "info", NOW - timedelta(hours=8)),
            ("metric_anomaly", "truenas:disk:rebound",     "Pool usage rebounding: 88.7% → 91.3% as nightly backups ran.", "high", NOW - timedelta(hours=2)),
            ("audit",          "admin:plan:expansion",     "Ordering 2x 16TB drives for raidz2 expansion. ETA 3 days.", "info",    NOW - timedelta(hours=1)),
        ],
        "alert_links": [("ial-nas-1", "rule-disk-full", "ai-truenas-disk", "truenas-scale:disk_percent", "trigger")],
    },
    # 2. Docker Container Storm (resolved, 3 days ago)
    {
        "id": "inc-container-storm", "title": "Docker Container Restart Storm — Image Pull Failure",
        "summary": "Bad image tag in docker-compose caused cascading container restarts on docker-prod. 14 of 22 containers entered restart loops. Root cause: registry.example.com rate-limited pulls. Fixed by pinning image digests and adding pull-through cache.",
        "status": "resolved", "severity": "high", "source": "alert_auto",
        "group_id": "grp-services", "primary_asset_id": "asset-docker",
        "assignee": "admin", "created_by": "system",
        "opened_at": NOW - timedelta(days=3),
        "mitigated_at": NOW - timedelta(days=3) + timedelta(hours=1),
        "resolved_at": NOW - timedelta(days=3) + timedelta(hours=4),
        "root_cause": "Container registry rate limit caused image pull failures during scheduled update. Compose restart policy cascaded failures across dependent containers.",
        "events": [
            ("alert_fired",    "docker-prod:cpu:spike",    "Alert fired: High CPU on docker-prod (94.2%)",              "high",    NOW - timedelta(days=3)),
            ("log_burst",      "docker-prod:logs:burst",   "Burst of 340 error logs in 5 minutes from container runtime.", "high", NOW - timedelta(days=3) + timedelta(minutes=2)),
            ("metric_anomaly", "docker-prod:cpu:sustained","CPU sustained above 90% — container restart loops detected.", "high",  NOW - timedelta(days=3) + timedelta(minutes=5)),
            ("audit",          "admin:triage:docker",       "14/22 containers in restart loop. Image pull errors in logs.", "info", NOW - timedelta(days=3) + timedelta(minutes=15)),
            ("audit",          "admin:analysis:registry",   "Registry returning 429 Too Many Requests. Rate limit hit during batch update.", "info", NOW - timedelta(days=3) + timedelta(minutes=25)),
            ("config_change",  "admin:fix:compose-down",    "docker compose down on affected stacks to stop restart loops.", "info", NOW - timedelta(days=3) + timedelta(minutes=30)),
            ("audit",          "admin:fix:pin-digests",     "Pinning all container images to digest references.",        "info",    NOW - timedelta(days=3) + timedelta(hours=1)),
            ("config_change",  "admin:fix:pull-cache",      "Deployed local registry pull-through cache to avoid future rate limits.", "info", NOW - timedelta(days=3) + timedelta(hours=2)),
            ("action_run",     "admin:restart:stacks",      "docker compose up -d: all 22 containers healthy.",          "info",   NOW - timedelta(days=3) + timedelta(hours=3)),
            ("alert_resolved", "docker-prod:cpu:resolved",  "CPU returned to normal (22%). All containers stable for 1 hour.", "info", NOW - timedelta(days=3) + timedelta(hours=4)),
        ],
        "alert_links": [("ial-docker-1", "rule-high-cpu", "ai-docker-cpu", "docker-prod:cpu_percent", "trigger")],
    },
    # 3. Network Outage (resolved, 5 days ago)
    {
        "id": "inc-network-outage", "title": "Network Outage — OPNsense Firmware Update Failure",
        "summary": "OPNsense firmware update from 24.1 to 24.7 caused kernel panic on reboot. All LAN traffic interrupted for 45 minutes. Recovered by booting previous kernel from console. Firmware update rescheduled with maintenance window.",
        "status": "resolved", "severity": "critical", "source": "manual",
        "group_id": "grp-network", "primary_asset_id": "asset-opnsense",
        "assignee": "admin", "created_by": "admin",
        "opened_at": NOW - timedelta(days=5),
        "mitigated_at": NOW - timedelta(days=5) + timedelta(minutes=45),
        "resolved_at": NOW - timedelta(days=5) + timedelta(hours=2),
        "root_cause": "OPNsense 24.7 firmware incompatible with Realtek NIC driver. Kernel panic on boot. Upstream bug confirmed in OPNsense forums.",
        "events": [
            ("config_change",  "admin:update:opnsense",    "Initiated OPNsense firmware update 24.1 → 24.7.",          "info",    NOW - timedelta(days=5)),
            ("heartbeat_change","opnsense:heartbeat:lost",  "OPNsense heartbeat lost after firmware update reboot.",    "critical",NOW - timedelta(days=5) + timedelta(minutes=2)),
            ("alert_fired",    "opnsense:offline",          "Alert fired: Node Offline for opnsense.",                  "critical",NOW - timedelta(days=5) + timedelta(minutes=7)),
            ("log_burst",      "hub:connectivity:burst",    "12 assets reported connectivity loss within 30 seconds.",  "critical",NOW - timedelta(days=5) + timedelta(minutes=8)),
            ("audit",          "admin:investigate:console",  "Connected to OPNsense via serial console. Kernel panic on NIC driver init.", "info", NOW - timedelta(days=5) + timedelta(minutes=15)),
            ("config_change",  "admin:fix:boot-previous",   "Booted previous kernel from recovery menu. Network restored.", "info", NOW - timedelta(days=5) + timedelta(minutes=45)),
            ("alert_resolved", "opnsense:online",           "OPNsense back online. All 12 assets reconnected.",        "info",    NOW - timedelta(days=5) + timedelta(minutes=48)),
            ("audit",          "admin:rollback:firmware",    "Rolled back to OPNsense 24.1. Firmware update postponed pending driver fix.", "info", NOW - timedelta(days=5) + timedelta(hours=1)),
            ("audit",          "admin:postmortem:network",   "Postmortem: Adding pre-update NIC driver compatibility check to runbook.", "info", NOW - timedelta(days=5) + timedelta(hours=2)),
        ],
        "alert_links": [("ial-opn-1", "rule-offline", None, "opnsense:heartbeat", "trigger"),
                         ("ial-opn-2", "rule-net-latency", "ai-opn-latency", "opnsense:net_latency", "symptom")],
    },
    # 4. K3s CrashLoop (investigating, 2h ago)
    {
        "id": "inc-k3s-crash", "title": "K3s cert-manager Webhook CrashLoopBackOff",
        "summary": "cert-manager-webhook pod entering CrashLoopBackOff after Helm chart upgrade. TLS certificate renewal for ingress resources blocked. Investigating compatibility between cert-manager v1.16 and K3s v1.31.",
        "status": "investigating", "severity": "medium", "source": "alert_auto",
        "group_id": "grp-services", "primary_asset_id": "asset-k3s-m",
        "assignee": "admin", "created_by": "system",
        "opened_at": NOW - timedelta(hours=2),
        "events": [
            ("alert_fired",    "k3s:cert-manager:crash",    "Alert fired: Service Down — cert-manager-webhook CrashLoopBackOff.", "critical", NOW - timedelta(hours=2)),
            ("log_burst",      "k3s-master:pods:errors",    "18 error logs from cert-manager namespace in 10 minutes.",  "high",   NOW - timedelta(hours=2) + timedelta(minutes=5)),
            ("audit",          "admin:investigate:k3s",       "Investigating. Helm upgrade to cert-manager 1.16 ran 2h ago. Webhook pod failing TLS handshake.", "info", NOW - timedelta(hours=1, minutes=30)),
            ("audit",          "admin:analysis:compat",       "K3s v1.31 admission webhook changes may be incompatible with cert-manager 1.16 conversion webhook.", "info", NOW - timedelta(hours=1)),
            ("audit",          "admin:plan:rollback",         "Planning Helm rollback to cert-manager 1.15.4 if no upstream fix found.", "info", NOW - timedelta(minutes=30)),
        ],
        "alert_links": [("ial-k3s-1", "rule-svc-down", "ai-k3s-crash", "k3s-master:cert-manager", "trigger")],
    },
    # 5. Dev Workstation Unreachable (open, 3h ago)
    {
        "id": "inc-dev-offline", "title": "Dev Workstation Unreachable — CI/CD Impact",
        "summary": "dev-workstation went offline 3 hours ago. Last heartbeat showed elevated disk I/O. GitLab CI jobs using this runner are queuing. Other runners absorbing load but pipeline times increased 3x.",
        "status": "open", "severity": "high", "source": "alert_auto",
        "group_id": "grp-dev", "primary_asset_id": "asset-dev-ws",
        "assignee": "", "created_by": "system",
        "opened_at": NOW - timedelta(hours=3),
        "events": [
            ("heartbeat_change","dev-ws:heartbeat:stale",   "Heartbeat status changed: online → stale → offline.",     "high",    NOW - timedelta(hours=3)),
            ("alert_fired",    "ai-dev-offline",             "Alert fired: Node Offline for dev-workstation.",           "high",    NOW - timedelta(hours=3) + timedelta(minutes=5)),
            ("audit",          "system:impact:cicd",          "GitLab CI queue depth increased. 6 jobs waiting for runner.", "info", NOW - timedelta(hours=2)),
            ("audit",          "admin:investigate:dev",        "Attempting SSH — connection refused. May need physical console access.", "info", NOW - timedelta(hours=1)),
        ],
        "alert_links": [("ial-dev-1", "rule-offline", "ai-dev-offline", "dev-workstation:heartbeat", "trigger")],
    },
]


# ════════════════════════════════════════════════════════════════
# 7. METRIC PROFILES per asset
# ════════════════════════════════════════════════════════════════

# (asset_id, metric, unit, base_value, variance, daily_amplitude, trend_per_day, spikes)
# spikes: list of (days_ago, duration_hours, spike_amount)
AUDIT_EVENTS = [
    ("login",            "admin", None,               "sess-001", "allow", "valid credentials",         {"ip":"192.168.1.100","method":"local"}, NOW - timedelta(hours=4)),
    ("login",            "admin", None,               "sess-002", "allow", "valid credentials",         {"ip":"192.168.1.100","method":"local"}, NOW - timedelta(days=1)),
    ("login",            "admin", None,               "sess-003", "allow", "valid credentials",         {"ip":"192.168.1.100","method":"local"}, NOW - timedelta(days=3)),
    ("login",            "admin", None,               "sess-004", "allow", "valid credentials",         {"ip":"10.10.1.50","method":"local","note":"VPN"}, NOW - timedelta(days=5)),
    ("login_failed",     None,    None,               None,       "deny",  "invalid password",          {"ip":"10.0.1.99","username":"root","method":"local"}, NOW - timedelta(days=2)),
    ("login_failed",     None,    None,               None,       "deny",  "unknown username",          {"ip":"10.0.1.99","username":"admin123","method":"local"}, NOW - timedelta(days=4)),
    ("asset_create",     "admin", "asset-mc",          "sess-002", "allow", "agent auto-registration",  {"name":"minecraft-server","type":"vm"}, NOW - timedelta(days=45)),
    ("asset_update",     "admin", "asset-truenas",     "sess-001", "allow", "tags updated",             {"added_tags":["iscsi"]}, NOW - timedelta(days=3)),
    ("asset_update",     "admin", "asset-docker",      "sess-001", "allow", "group assignment changed", {"old_group":"grp-prod","new_group":"grp-services"}, NOW - timedelta(days=10)),
    ("alert_rule_create","admin", "rule-high-cpu",     "sess-003", "allow", "rule created",             {"name":"High CPU Usage","severity":"medium"}, NOW - timedelta(days=60)),
    ("alert_rule_create","admin", "rule-disk-full",    "sess-003", "allow", "rule created",             {"name":"Disk Nearly Full","severity":"critical"}, NOW - timedelta(days=60)),
    ("alert_rule_create","admin", "rule-cert-expiry",  "sess-002", "allow", "rule created",             {"name":"Certificate Expiring","severity":"high"}, NOW - timedelta(days=30)),
    ("alert_rule_create","admin", "rule-backup-stale", "sess-002", "allow", "rule created",             {"name":"Backup Stale","severity":"high"}, NOW - timedelta(days=30)),
    ("alert_rule_create","admin", "rule-net-latency",  "sess-004", "allow", "rule created",             {"name":"Network Latency High","severity":"medium"}, NOW - timedelta(days=14)),
    ("alert_ack",        "admin", "ai-docker-cpu",     "sess-001", "allow", "alert acknowledged",       {"rule":"rule-high-cpu","asset":"asset-docker"}, NOW - timedelta(hours=18)),
    ("incident_create",  "system","inc-nas-storage",   None,       "allow", "auto-created from alert",  {"alert":"ai-truenas-disk","severity":"critical"}, NOW - timedelta(hours=18)),
    ("incident_create",  "system","inc-container-storm",None,      "allow", "auto-created from alert",  {"alert":"ai-docker-cpu","severity":"high"}, NOW - timedelta(days=3)),
    ("incident_create",  "admin", "inc-network-outage","sess-004", "allow", "manually created",         {"severity":"critical"}, NOW - timedelta(days=5)),
    ("incident_create",  "system","inc-k3s-crash",     None,       "allow", "auto-created from alert",  {"alert":"ai-k3s-crash","severity":"medium"}, NOW - timedelta(hours=2)),
    ("incident_create",  "system","inc-dev-offline",   None,       "allow", "auto-created from alert",  {"alert":"ai-dev-offline","severity":"high"}, NOW - timedelta(hours=3)),
    ("incident_update",  "admin", "inc-k3s-crash",     "sess-001", "allow", "status → investigating",   {"old":"open","new":"investigating"}, NOW - timedelta(hours=1, minutes=30)),
    ("incident_resolve", "admin", "inc-container-storm","sess-003","allow", "incident resolved",        {"resolution":"image digests pinned, pull-through cache deployed"}, NOW - timedelta(days=3) + timedelta(hours=4)),
    ("incident_resolve", "admin", "inc-network-outage","sess-004", "allow", "incident resolved",        {"resolution":"rolled back to OPNsense 24.1"}, NOW - timedelta(days=5) + timedelta(hours=2)),
    ("group_create",     "admin", "grp-prod",          "sess-003", "allow", "group created",            {"name":"Production"}, NOW - timedelta(days=90)),
    ("group_create",     "admin", "grp-compute",       "sess-003", "allow", "group created",            {"name":"Compute","parent":"grp-prod"}, NOW - timedelta(days=90)),
    ("group_create",     "admin", "grp-storage",       "sess-003", "allow", "group created",            {"name":"Storage","parent":"grp-prod"}, NOW - timedelta(days=90)),
    ("group_create",     "admin", "grp-network",       "sess-003", "allow", "group created",            {"name":"Network","parent":"grp-prod"}, NOW - timedelta(days=90)),
    ("group_create",     "admin", "grp-services",      "sess-003", "allow", "group created",            {"name":"Services","parent":"grp-prod"}, NOW - timedelta(days=90)),
    ("group_create",     "admin", "grp-dev",           "sess-003", "allow", "group created",            {"name":"Development"}, NOW - timedelta(days=90)),
    ("group_create",     "admin", "grp-remote",        "sess-003", "allow", "group created",            {"name":"Remote"}, NOW - timedelta(days=90)),
    ("settings_update",  "admin", "retention_policy",  "sess-001", "allow", "metric retention changed", {"old_days":90,"new_days":30}, NOW - timedelta(days=7)),
    ("settings_update",  "admin", "backup_schedule",   "sess-002", "allow", "backup schedule updated",  {"old":"daily 02:00","new":"daily 01:00"}, NOW - timedelta(days=14)),
]


# ════════════════════════════════════════════════════════════════
# 10. ACTION RUNS — 15 realistic commands
# ════════════════════════════════════════════════════════════════

ACTION_RUNS = [
    ("restart_service", "admin", "asset-docker",  "docker restart nginx-proxy",                      "completed", "nginx-proxy\n", "", NOW - timedelta(minutes=45), 30),
    ("run_script",      "admin", "asset-truenas", "zpool status tank",                                "completed", "pool: tank\n  state: ONLINE\n  scan: scrub repaired 0B in 06:14:22\n  config:\n    NAME        STATE\n    tank        ONLINE\n      raidz2-0  ONLINE\n", "", NOW - timedelta(hours=2), 5),
    ("run_script",      "admin", "asset-truenas", "zfs list -o name,used,avail,refer tank",           "completed", "NAME              USED   AVAIL  REFER\ntank              82.4T  8.22T  256K\ntank/backups      28.1T  8.22T  28.1T\ntank/vms          42.3T  8.22T  42.3T\ntank/media        12.0T  8.22T  12.0T\n", "", NOW - timedelta(hours=1, minutes=45), 3),
    ("restart_service", "admin", "asset-k3s-m",   "kubectl rollout restart deployment/cert-manager-webhook -n cert-manager", "failed", "", "error: deployment cert-manager-webhook restart failed: pods not ready after 60s", NOW - timedelta(hours=1), 65),
    ("run_script",      "admin", "asset-k3s-m",   "kubectl get pods -n cert-manager",                 "completed", "NAME                                       READY   STATUS             RESTARTS   AGE\ncert-manager-7f9b4c8d6-abc12              1/1     Running            0          2h\ncert-manager-cainjector-6c8d9b4-def34     1/1     Running            0          2h\ncert-manager-webhook-7f9b4c8d6-x2k4m      0/1     CrashLoopBackOff   12         2h\n", "", NOW - timedelta(minutes=50), 4),
    ("run_script",      "admin", "asset-pve1",    "qm list",                                          "completed", "VMID NAME              STATUS     MEM(MB)    BOOTDISK(GB) PID\n100  docker-prod       running    16384      100.00       1234\n101  k3s-master        running    8192       50.00        2345\n102  gitlab-runner     running    4096       40.00        3456\n103  dev-workstation   stopped    32768      200.00       0\n", "", NOW - timedelta(hours=3), 2),
    ("restart_service", "admin", "asset-docker",  "docker compose -f /opt/stacks/monitoring/docker-compose.yml restart", "completed", "Restarting grafana ... done\nRestarting prometheus ... done\nRestarting loki ... done\n", "", NOW - timedelta(days=1), 15),
    ("run_script",      "admin", "asset-pbs",     "proxmox-backup-manager task list --limit 5",        "completed", "UPID                                    ENDTIME  STATUS\ntask-backup-vm100-2026_04_16-01:00  01:42    OK\ntask-backup-vm101-2026_04_16-01:00  01:38    OK\ntask-backup-vm102-2026_04_16-01:00  01:25    OK\ntask-verify-2026_04_16-02:00        02:14    OK\ntask-gc-2026_04_15-03:00            03:22    OK\n", "", NOW - timedelta(hours=5), 3),
    ("run_script",      "admin", "asset-opnsense","pfctl -sr | wc -l",                                "completed", "142\n", "", NOW - timedelta(days=1), 2),
    ("run_script",      "admin", "asset-pihole",  "pihole -c -e",                                     "completed", "Queries: 14231 (16.8% blocked)\nBlocked: 2391\nBlocklist: 152891 domains\n", "", NOW - timedelta(hours=1), 3),
    ("run_script",      "admin", "asset-offsite", "wg show wg0",                                      "completed", "interface: wg0\n  public key: abc123...\n  listening port: 51820\n\npeer: def456...\n  endpoint: 10.0.1.254:51820\n  allowed ips: 10.0.1.0/24\n  latest handshake: 12 seconds ago\n  transfer: 42.31 GiB received, 1.24 GiB sent\n", "", NOW - timedelta(hours=4), 2),
    ("run_script",      "admin", "asset-docker",  "docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}'", "completed", "NAME               CPU %     MEM USAGE / LIMIT\nnginx-proxy        0.12%     48MiB / 512MiB\ngrafana            2.34%     256MiB / 1GiB\nprometheus         4.56%     1.2GiB / 2GiB\nloki               1.23%     512MiB / 1GiB\nvaultwarden        0.05%     32MiB / 256MiB\nauthelia           0.08%     64MiB / 256MiB\n", "", NOW - timedelta(minutes=20), 5),
    ("restart_service", "admin", "asset-hass",    "ha core restart",                                   "completed", "Core restarting...\nCore started successfully\n", "", NOW - timedelta(days=2), 30),
    ("run_script",      "admin", "asset-mc",      "rcon-cli list",                                     "completed", "There are 0/20 players online:\n", "", NOW - timedelta(hours=1), 1),
    ("run_script",      "admin", "asset-pve2",    "pvecm status",                                      "completed", "Cluster information\n==================\nName:             lab-cluster\nConfig Version:   4\nNodes:            2\nQuorum:           2\nCluster ID:       12345\n", "", NOW - timedelta(days=1), 3),
]


# ════════════════════════════════════════════════════════════════
# SQL OUTPUT
# ════════════════════════════════════════════════════════════════

def main():
    out = sys.stdout

    out.write("-- LabTether Demo Seed Data\n")
    out.write("-- Generated by scripts/generate-demo-seed.py\n")
    out.write(f"-- Generated at: {NOW.isoformat()}\n")
    out.write("-- Safe to re-run: TRUNCATEs demo tables first.\n\n")
    out.write("BEGIN;\n\n")

    # Truncate
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- TRUNCATE\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("TRUNCATE\n")
    out.write("    zone_members, topology_connections, topology_zones, topology_layouts,\n")
    out.write("    asset_edges, metric_samples, alert_instances, alert_rules,\n")
    out.write("    incident_events, incident_alert_links, incidents,\n")
    out.write("    log_events, audit_events, groups, action_runs, assets\n")
    out.write("CASCADE;\n\n")

    # Groups
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- GROUPS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO groups (id, name, slug, parent_group_id, icon, sort_order, timezone, location, metadata, created_at, updated_at) VALUES\n")
    rows = []
    for gid, name, slug, parent, icon, sort in GROUPS:
        parent_sql = sql_str(parent) if parent else "NULL"
        created = ts(NOW - timedelta(days=90))
        rows.append(f"    ({sql_str(gid)}, {sql_str(name)}, {sql_str(slug)}, {parent_sql}, {sql_str(icon)}, {sort}, 'America/New_York', '', '{{}}'::jsonb, '{created}', '{created}')")
    out.write(",\n".join(rows))
    out.write("\nON CONFLICT (id) DO NOTHING;\n\n")

    # Assets
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- ASSETS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO assets (id, type, name, source, status, platform, host, transport_type, group_id, tags, metadata, created_at, updated_at, last_seen_at) VALUES\n")
    rows = []
    for aid, atype, name, source, status, platform, host, transport, group_id, tags, metadata, created_days, last_seen_sec in ASSETS:
        created = ts(NOW - timedelta(days=created_days))
        updated = ts(NOW)
        last_seen = ts(NOW - timedelta(seconds=last_seen_sec))
        rows.append(f"    ({sql_str(aid)}, {sql_str(atype)}, {sql_str(name)}, {sql_str(source)}, {sql_str(status)}, {sql_str(platform)}, {sql_str(host)}, {sql_str(transport)}, {sql_str(group_id)}, {sql_json(tags)}, {sql_json(metadata)}, '{created}', '{updated}', '{last_seen}')")
    out.write(",\n".join(rows))
    out.write("\nON CONFLICT (id) DO NOTHING;\n\n")

    # Topology edges
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- TOPOLOGY EDGES\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO asset_edges (id, source_asset_id, target_asset_id, relationship_type, direction, criticality, origin, confidence, match_signals, metadata, created_at, updated_at) VALUES\n")
    rows = []
    for src, tgt, rel, direction, crit, origin, note in EDGES:
        eid = uid()
        conf = "0.85" if origin == "suggested" else "1.0"
        created = ts(NOW - timedelta(days=random.randint(30, 180)))
        rows.append(f"    ({sql_str(eid)}, {sql_str(src)}, {sql_str(tgt)}, {sql_str(rel)}, {sql_str(direction)}, {sql_str(crit)}, {sql_str(origin)}, {conf}, NULL, {sql_json({'note': note})}, '{created}', '{created}')")
    out.write(",\n".join(rows))
    out.write("\nON CONFLICT DO NOTHING;\n\n")

    # Topology layout — zones with positioned assets
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- TOPOLOGY LAYOUT\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")

    topo_id = "00000000-0000-0000-0000-000000000001"
    viewport = json.dumps({"x": 100, "y": 50, "zoom": 0.75})
    out.write(f"INSERT INTO topology_layouts (id, name, viewport, created_at, updated_at) VALUES\n")
    out.write(f"    ('{topo_id}', 'My Homelab', '{viewport}'::jsonb, NOW(), NOW())\n")
    out.write("ON CONFLICT (id) DO NOTHING;\n\n")

    # Zone layout — wide spacing, large zones, no overlap.
    # Grid with generous gaps so connections don't cross zones.
    #
    #  [ Compute ]        [ Storage ]
    #  (0,0 500x240)      (600,0 500x240)
    #
    #  [        Services (wide)          ]  [ Network ]
    #  (0,340 840x500)                      (940,340 400x300)
    #
    #  [ Dev ]            [ Remote ]
    #  (0,940 500x240)    (600,940 400x200)
    #
    zid_compute  = "10000000-0000-0000-0000-000000000002"
    zid_storage  = "10000000-0000-0000-0000-000000000003"
    zid_services = "10000000-0000-0000-0000-000000000004"
    zid_network  = "10000000-0000-0000-0000-000000000001"
    zid_dev      = "10000000-0000-0000-0000-000000000005"
    zid_remote   = "10000000-0000-0000-0000-000000000006"

    zones = [
        (zid_compute,  None, "Compute",  "purple", "cpu",      0, 0,      500, 240),
        (zid_storage,  None, "Storage",  "orange", "database", 600, 0,    500, 240),
        (zid_services, None, "Services", "green",  "layers",   0, 340,    840, 500),
        (zid_network,  None, "Network",  "blue",   "globe",    940, 340,  400, 300),
        (zid_dev,      None, "Dev",      "cyan",   "code",     0, 940,    500, 240),
        (zid_remote,   None, "Remote",   "gray",   "radio",    600, 940,  400, 200),
    ]

    out.write("INSERT INTO topology_zones (id, topology_id, parent_zone_id, label, color, icon, position, size, sort_order, created_at, updated_at) VALUES\n")
    zone_rows = []
    for zid, parent, label, color, icon, x, y, w, h in zones:
        parent_sql = f"'{parent}'" if parent else "NULL"
        zone_rows.append(f"    ('{zid}', '{topo_id}', {parent_sql}, {sql_str(label)}, {sql_str(color)}, {sql_str(icon)}, {sql_json({'x':x,'y':y})}, {sql_json({'width':w,'height':h})}, 0, NOW(), NOW())")
    out.write(",\n".join(zone_rows))
    out.write("\nON CONFLICT (id) DO NOTHING;\n\n")

    # Place assets in zones with positions relative to zone top-left.
    # Each asset gets an (x, y) offset within its zone.
    zone_members = [
        # Compute zone (500x240) — top left
        (zid_compute, "asset-pve1",     60, 80),
        (zid_compute, "asset-pve2",     280, 80),
        # Storage zone (500x240) — top right
        (zid_storage, "asset-truenas",  60, 80),
        (zid_storage, "asset-pbs",      280, 80),
        # Services zone (840x500) — middle left, 3-column grid
        (zid_services,"asset-docker",   60, 70),
        (zid_services,"asset-k3s-m",    320, 70),
        (zid_services,"asset-k3s-w1",   580, 70),
        (zid_services,"asset-hass",     60, 200),
        (zid_services,"asset-media",    320, 200),
        (zid_services,"asset-mon",      580, 200),
        (zid_services,"asset-mc",       160, 340),
        (zid_services,"asset-htpc",     420, 340),
        # Network zone (400x300) — middle right
        (zid_network, "asset-opnsense", 60, 70),
        (zid_network, "asset-pihole",   220, 70),
        (zid_network, "asset-unifi",    140, 180),
        # Dev zone (500x240) — bottom left
        (zid_dev,     "asset-gitlab",   60, 80),
        (zid_dev,     "asset-dev-ws",   280, 80),
        # Remote zone (400x200) — bottom right
        (zid_remote,  "asset-offsite",  140, 70),
    ]

    out.write("INSERT INTO zone_members (zone_id, asset_id, position, sort_order) VALUES\n")
    zm_rows = []
    for zid, aid, x, y in zone_members:
        zm_rows.append(f"    ('{zid}', {sql_str(aid)}, {sql_json({'x':x,'y':y})}, 0)")
    out.write(",\n".join(zm_rows))
    out.write("\nON CONFLICT (zone_id, asset_id) DO NOTHING;\n\n")

    # Topology connections — only the key relationships to keep it clean.
    # Rule: max 1-2 connections per node to avoid spaghetti.
    topo_connections = [
        # Compute → Services (VMs host the services)
        ("asset-pve1",    "asset-docker",  "runs_on",      ""),
        ("asset-pve1",    "asset-k3s-m",   "runs_on",      ""),
        ("asset-pve2",    "asset-k3s-w1",  "runs_on",      ""),
        # Storage → Compute (NFS backing)
        ("asset-truenas", "asset-pve1",    "provides_to",  ""),
        ("asset-truenas", "asset-pve2",    "provides_to",  ""),
        # Backup chain
        ("asset-pbs",     "asset-offsite", "provides_to",  ""),
        # K3s cluster
        ("asset-k3s-m",   "asset-k3s-w1", "contains",     ""),
        # Compute → Dev
        ("asset-pve1",    "asset-gitlab",  "runs_on",      ""),
    ]

    out.write("INSERT INTO topology_connections (id, topology_id, source_asset_id, target_asset_id, relationship, user_defined, label, created_at) VALUES\n")
    tc_rows = []
    for src, tgt, rel, label in topo_connections:
        tc_rows.append(f"    ('{uid()}', '{topo_id}', {sql_str(src)}, {sql_str(tgt)}, {sql_str(rel)}, true, {sql_str(label)}, NOW())")
    out.write(",\n".join(tc_rows))
    out.write("\nON CONFLICT DO NOTHING;\n\n")

    # Alert rules
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- ALERT RULES\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO alert_rules (id, name, description, status, kind, severity, target_scope, cooldown_seconds, reopen_after_seconds, evaluation_interval_seconds, window_seconds, condition, labels, metadata, created_by, created_at, updated_at) VALUES\n")
    rows = []
    for rid, name, desc, status, kind, sev, scope, cool, reopen, eval_int, window, condition, labels in ALERT_RULES:
        created = ts(NOW - timedelta(days=60))
        rows.append(f"    ({sql_str(rid)}, {sql_str(name)}, {sql_str(desc)}, {sql_str(status)}, {sql_str(kind)}, {sql_str(sev)}, {sql_str(scope)}, {cool}, {reopen}, {eval_int}, {window}, {sql_json(condition)}, {sql_json(labels)}, '{{}}'::jsonb, 'admin', '{created}', '{created}')")
    out.write(",\n".join(rows))
    out.write("\nON CONFLICT (id) DO NOTHING;\n\n")

    # Alert instances
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- ALERT INSTANCES\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO alert_instances (id, rule_id, fingerprint, status, severity, labels, annotations, started_at, resolved_at, last_fired_at, suppressed_by, created_at, updated_at) VALUES\n")
    rows = []
    for aiid, rule, fp, status, sev, labels, annotations, started, resolved, last_fired in ALERT_INSTANCES:
        rows.append(f"    ({sql_str(aiid)}, {sql_str(rule)}, {sql_str(fp)}, {sql_str(status)}, {sql_str(sev)}, {sql_json(labels)}, {sql_json(annotations)}, '{ts(started)}', {sql_null_or_ts(resolved)}, '{ts(last_fired)}', NULL, '{ts(started)}', '{ts(last_fired)}')")
    out.write(",\n".join(rows))
    out.write("\nON CONFLICT (id) DO NOTHING;\n\n")

    # Incidents
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- INCIDENTS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    for inc in INCIDENTS:
        out.write(f"INSERT INTO incidents (id, title, summary, status, severity, source, group_id, primary_asset_id, assignee, created_by, opened_at, mitigated_at, resolved_at, closed_at, root_cause, metadata, created_at, updated_at) VALUES\n")
        mitigated = inc.get("mitigated_at")
        resolved = inc.get("resolved_at")
        root_cause = inc.get("root_cause", "")
        out.write(f"    ({sql_str(inc['id'])}, {sql_str(inc['title'])}, {sql_str(inc['summary'])}, {sql_str(inc['status'])}, {sql_str(inc['severity'])}, {sql_str(inc['source'])}, {sql_str(inc['group_id'])}, {sql_str(inc['primary_asset_id'])}, {sql_str(inc['assignee'])}, {sql_str(inc['created_by'])}, '{ts(inc['opened_at'])}', {sql_null_or_ts(mitigated)}, {sql_null_or_ts(resolved)}, NULL, {sql_str(root_cause)}, '{{}}'::jsonb, '{ts(inc['opened_at'])}', '{ts(resolved or mitigated or NOW)}')")
        out.write("\nON CONFLICT (id) DO NOTHING;\n\n")

    # Incident events
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- INCIDENT EVENTS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    for inc in INCIDENTS:
        for event_type, source_ref, summary, severity, occurred_at in inc["events"]:
            eid = uid()
            out.write(f"INSERT INTO incident_events (id, incident_id, event_type, source_ref, summary, severity, metadata, occurred_at, created_at) VALUES\n")
            out.write(f"    ({sql_str(eid)}, {sql_str(inc['id'])}, {sql_str(event_type)}, {sql_str(source_ref)}, {sql_str(summary)}, {sql_str(severity)}, '{{}}'::jsonb, '{ts(occurred_at)}', '{ts(occurred_at)}')")
            out.write("\nON CONFLICT DO NOTHING;\n")
    out.write("\n")

    # Incident alert links
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- INCIDENT ALERT LINKS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    for inc in INCIDENTS:
        for link_id, rule_id, instance_id, fingerprint, link_type in inc.get("alert_links", []):
            rule_sql = sql_str(rule_id) if rule_id else "NULL"
            inst_sql = sql_str(instance_id) if instance_id else "NULL"
            fp_sql = sql_str(fingerprint) if fingerprint else "NULL"
            out.write(f"INSERT INTO incident_alert_links (id, incident_id, alert_rule_id, alert_instance_id, alert_fingerprint, link_type, created_by, created_at) VALUES\n")
            out.write(f"    ({sql_str(link_id)}, {sql_str(inc['id'])}, {rule_sql}, {inst_sql}, {fp_sql}, {sql_str(link_type)}, 'system', '{ts(inc['opened_at'])}')")
            out.write("\nON CONFLICT (id) DO NOTHING;\n")
    out.write("\n")

    # Metrics — the big one
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- METRIC SAMPLES (7 days, 15-min intervals)\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    all_metrics = []
    for asset_id, metric, unit, base, variance, daily_amp, trend, spikes in METRIC_PROFILES:
        all_metrics.extend(generate_metric_series(asset_id, metric, unit, base, variance, daily_amp, trend, spikes, now=NOW))

    # Write in batches of 500 for performance
    batch_size = 500
    for i in range(0, len(all_metrics), batch_size):
        batch = all_metrics[i:i + batch_size]
        out.write("INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at) VALUES\n")
        rows = []
        for asset_id, metric, unit, value, collected_at in batch:
            rows.append(f"    ({sql_str(asset_id)}, {sql_str(metric)}, {sql_str(unit)}, {value}, '{ts(collected_at)}')")
        out.write(",\n".join(rows))
        out.write(";\n")
    out.write(f"-- Total metric samples: {len(all_metrics)}\n\n")

    # Log events
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- LOG EVENTS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    logs = generate_logs(NOW, uid)
    out.write("INSERT INTO log_events (id, asset_id, source, level, message, fields, timestamp) VALUES\n")
    rows = []
    for log_id, asset_id, source, level, message, fields, timestamp in logs:
        asset_sql = sql_str(asset_id) if asset_id else "NULL"
        fields_sql = sql_json(fields) if fields else "NULL"
        rows.append(f"    ({sql_str(log_id)}, {asset_sql}, {sql_str(source)}, {sql_str(level)}, {sql_str(message)}, {fields_sql}, '{ts(timestamp)}')")
    out.write(",\n".join(rows))
    out.write(";\n")
    out.write(f"-- Total log events: {len(logs)}\n\n")

    # Audit events
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- AUDIT EVENTS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO audit_events (id, type, actor_id, target, session_id, decision, reason, details, timestamp) VALUES\n")
    rows = []
    for atype, actor, target, session, decision, reason, details, timestamp in AUDIT_EVENTS:
        rows.append(f"    ({sql_str(uid())}, {sql_str(atype)}, {sql_null_or_str(actor)}, {sql_null_or_str(target)}, {sql_null_or_str(session)}, {sql_null_or_str(decision)}, {sql_null_or_str(reason)}, {sql_json(details)}, '{ts(timestamp)}')")
    out.write(",\n".join(rows))
    out.write(";\n\n")

    # Action runs
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("-- ACTION RUNS\n")
    out.write("-- ════════════════════════════════════════════════════════════════\n")
    out.write("INSERT INTO action_runs (id, type, actor_id, target, command, status, output, error, created_at, updated_at, completed_at) VALUES\n")
    rows = []
    for atype, actor, target, command, status, output, error, created_at, duration_sec in ACTION_RUNS:
        completed = created_at + timedelta(seconds=duration_sec)
        rows.append(f"    ({sql_str(uid())}, {sql_str(atype)}, {sql_str(actor)}, {sql_str(target)}, {sql_str(command)}, {sql_str(status)}, {sql_str(output)}, {sql_str(error)}, '{ts(created_at)}', '{ts(completed)}', '{ts(completed)}')")
    out.write(",\n".join(rows))
    out.write(";\n\n")

    out.write("COMMIT;\n")
    sys.stderr.write(f"Generated: {len(ASSETS)} assets, {len(EDGES)} edges, {len(ALERT_RULES)} rules, {len(ALERT_INSTANCES)} alerts, {len(INCIDENTS)} incidents, {len(all_metrics)} metrics, {len(logs)} logs, {len(AUDIT_EVENTS)} audit events, {len(ACTION_RUNS)} actions\n")


if __name__ == "__main__":
    main()
