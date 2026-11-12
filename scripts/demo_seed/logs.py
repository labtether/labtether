"""Log lines from the hosts over the last week, with bursts around the two past outages."""

import random

from .fleet import check_asset
from .sqlgen import ago, insert, section, uid

# (asset, source, level, message, fields, how long ago)
# Field values are strings: the hub reads log fields as text.
RECENT = [
    ("pve01", "pvedaemon", "info", "VM 100 (docker01): backup finished", None, dict(hours=7, minutes=10)),
    ("pve01", "pvedaemon", "info", "VM 102 (ci-runner): backup finished", None, dict(hours=7, minutes=2)),
    ("pve01", "smartd", "warning", "Device: /dev/sdc [SAT], 12 Currently unreadable (pending) sectors",
     {"device": "/dev/sdc", "pending_sectors": "12"}, dict(hours=2)),
    ("pve01", "zed", "info", "Scrub of pool rpool finished with 0 errors", {"pool": "rpool"}, dict(days=3, hours=4)),
    ("pve01", "pveproxy", "info", "Login from 10.0.10.73 for user root@pam", None, dict(hours=5)),
    ("pve02", "pvedaemon", "info", "VM 113 (media01): backup finished", None, dict(hours=6, minutes=50)),
    ("pve02", "qm", "info", "Migration of VM 104 (minecraft) to pve01 finished in 12 seconds", None, dict(days=2)),
    ("pve02", "kernel", "warning", "nfs: server nas01 not responding, still trying", None, dict(days=5, minutes=-3)),

    ("nas01", "middlewared", "error", "Pool tank is 91% full. Performance drops above 90%.",
     {"pool": "tank", "used_percent": "91"}, dict(minutes=6)),
    ("nas01", "zed", "info", "Scrub of pool tank finished with 0 errors after 6h 14m",
     {"pool": "tank"}, dict(hours=8)),
    ("nas01", "smartd", "warning", "Device: /dev/sde [SAT], Temperature 49 Celsius reached limit of 50 Celsius",
     {"device": "/dev/sde", "temperature_celsius": "49"}, dict(hours=3)),
    ("nas01", "middlewared", "info", "Snapshot task created tank/vms@auto-daily", {"dataset": "tank/vms"}, dict(hours=6)),
    ("nas01", "middlewared", "info", "Snapshot retention on tank/vms changed from 30 days to 14 days",
     {"dataset": "tank/vms"}, dict(hours=12)),
    ("nas01", "middlewared", "info", "Snapshot retention on tank/backups changed from 30 days to 14 days",
     {"dataset": "tank/backups"}, dict(hours=12, minutes=-1)),
    ("nas01", "middlewared", "warning", "Pool tank is 88% full", {"pool": "tank", "used_percent": "88"}, dict(days=2)),

    ("pbs01", "proxmox-backup-proxy", "info", "Backup of vm/100 finished: 18.6 GiB in 9m 12s", {"guest": "vm/100"}, dict(hours=7, minutes=10)),
    ("pbs01", "proxmox-backup-proxy", "info", "Verify of datastore main finished: 8 snapshots OK", {"datastore": "main"}, dict(hours=6)),
    ("pbs01", "proxmox-backup-proxy", "info", "Prune of datastore main removed 12 snapshots", {"datastore": "main"}, dict(hours=5, minutes=40)),
    ("pbs01", "proxmox-backup-proxy", "info", "Garbage collection of datastore main freed 24.3 GiB", {"datastore": "main"}, dict(hours=5, minutes=20)),
    ("pbs01", "proxmox-backup-proxy", "info", "Sync job to offsite-pbs started", None, dict(hours=6, minutes=30)),

    ("vyos", "kernel", "info", "wg0: peer offsite-pbs handshake completed", {"interface": "wg0"}, dict(minutes=2)),
    ("vyos", "dhcpd", "info", "DHCPACK on 10.0.10.143 to kitchen-display via eth1", None, dict(minutes=18)),
    ("vyos", "kernel", "warning", "[WAN-IN-default-D] IN=eth0 SRC=198.51.100.23 DST=203.0.113.7 PROTO=TCP DPT=22",
     {"interface": "eth0", "rule": "WAN-IN default drop"}, dict(minutes=41)),
    ("vyos", "vyos-configd", "info", "Configuration saved by user vyos", None, dict(days=5, hours=-1)),

    ("pihole", "pihole-FTL", "info", "Gravity list updated: 152891 domains", None, dict(hours=12)),
    ("pihole", "pihole-FTL", "warning", "Rate limit: 10.0.10.31 made 1000 queries in 60 seconds",
     {"client": "10.0.10.31"}, dict(hours=2)),
    ("pihole", "dnsmasq-dhcp", "info", "DHCPACK(eth0) 10.0.10.151 kitchen-display", None, dict(minutes=35)),

    ("docker01", "dockerd", "warning", "Container paperless-worker exited with code 1, restarting (attempt 6)",
     {"container": "paperless-worker", "exit_code": "1"}, dict(minutes=4)),
    ("docker01", "dockerd", "warning", "Container paperless-worker exited with code 1, restarting (attempt 5)",
     {"container": "paperless-worker", "exit_code": "1"}, dict(minutes=9)),
    ("docker01", "dockerd", "warning", "Container paperless-worker exited with code 1, restarting (attempt 4)",
     {"container": "paperless-worker", "exit_code": "1"}, dict(minutes=14)),
    ("docker01", "paperless-worker", "error", "redis.exceptions.ConnectionError: connection refused, paperless-broker:6379",
     {"container": "paperless-worker"}, dict(minutes=4, seconds=5)),
    ("docker01", "dockerd", "info", "Container vaultwarden health status changed to healthy",
     {"container": "vaultwarden"}, dict(hours=1)),
    ("docker01", "dockerd", "info", "Image prune removed 14 images and freed 4.1 GB", None, dict(hours=6)),
    ("docker01", "sshd", "info", "Accepted publickey for admin from 10.0.10.73 port 51844", None, dict(hours=1, minutes=5)),

    ("k3s-cp1", "k3s", "error", "Back-off restarting failed container webhook in pod cert-manager-webhook-7f9b4c8d6-x2k4m",
     {"namespace": "cert-manager", "pod": "cert-manager-webhook-7f9b4c8d6-x2k4m"}, dict(minutes=5)),
    ("k3s-cp1", "k3s", "error", "failed calling webhook webhook.cert-manager.io: connection refused",
     {"namespace": "cert-manager"}, dict(minutes=25)),
    ("k3s-cp1", "k3s", "info", "Deployment ingress-nginx-controller has 3 of 3 replicas ready",
     {"namespace": "ingress-nginx"}, dict(hours=1)),
    ("k3s-cp1", "k3s", "info", "Helm upgrade of cert-manager to chart 1.16.2 finished", {"namespace": "cert-manager"}, dict(hours=2, minutes=3)),
    ("k3s-w1", "k3s", "warning", "Volume pvc-prometheus-data is 89% full", {"pvc": "pvc-prometheus-data"}, dict(hours=2)),
    ("k3s-w1", "k3s", "info", "Node k3s-w1 is Ready", None, dict(days=1)),

    ("homeassistant", "homeassistant", "warning", "Z-Wave node 14 (front door lock) is not responding",
     {"node": "14"}, dict(hours=1)),
    ("homeassistant", "homeassistant", "error", "Hue bridge at 10.0.10.90 refused the connection, retrying in 30 seconds",
     {"integration": "hue"}, dict(minutes=20)),
    ("homeassistant", "homeassistant", "info", "Automation lights_off_away ran", {"automation": "lights_off_away"}, dict(hours=3)),
    ("homeassistant", "supervisor", "info", "Backup core_2026.11.1 created", None, dict(hours=9)),

    ("media01", "dockerd", "info", "Container sonarr health status changed to healthy", {"container": "sonarr"}, dict(hours=4)),
    ("media01", "jellyfin", "warning", "Transcode of stream 2 fell behind playback on client living-room-tv",
     {"container": "jellyfin"}, dict(minutes=30)),
    ("media01", "jellyfin", "info", "Library scan finished: 142 new items", {"container": "jellyfin"}, dict(hours=6)),

    ("monitor01", "prometheus", "warning", "Scrape of target workstation:9100 failed: context deadline exceeded",
     {"container": "prometheus"}, dict(minutes=3)),
    ("monitor01", "grafana", "info", "Dashboard Home lab overview saved by admin", {"container": "grafana"}, dict(days=1, hours=2)),
    ("monitor01", "loki", "info", "Compaction finished: 48 GB stored, 14 days kept", {"container": "loki"}, dict(hours=4)),

    ("ci-runner", "gitea-runner", "warning", "Job 1847 (test) timed out after 30 minutes", {"job": "1847"}, dict(hours=4)),
    ("ci-runner", "gitea-runner", "info", "Job 1852 (build) finished in 7m 48s", {"job": "1852"}, dict(minutes=30)),
    ("ci-runner", "gitea-runner", "info", "Runner cache cleaned: 2.8 GB kept", None, dict(hours=2)),

    ("workstation", "gitea-runner", "info", "Job 1849 (lint) finished in 1m 12s", {"job": "1849"}, dict(hours=3, minutes=12)),
    ("workstation", "kernel", "warning", "nvme0n1: I/O 212 QID 5 timeout, completion polled",
     {"device": "nvme0n1"}, dict(hours=3, minutes=5)),
    ("workstation", "labtether-agent", "error", "Connection to the hub lost, reconnecting", None, dict(hours=3, minutes=1)),
    ("workstation", "systemd", "info", "Reached target Suspend", None, dict(hours=3)),

    ("minecraft", "minecraft", "info", "Saved the game", None, dict(minutes=5)),
    ("minecraft", "minecraft", "info", "World backup written: 4.2 GB", None, dict(hours=6)),
    ("minecraft", "minecraft", "warning", "Can't keep up! Is the server overloaded? Running 2104ms or 42 ticks behind",
     None, dict(hours=10)),

    ("offsite-pbs", "proxmox-backup-proxy", "info", "Sync from pbs01 finished: 42.3 GiB in 3h 12m", None, dict(hours=5)),
    ("offsite-pbs", "proxmox-backup-proxy", "info", "Verify of datastore offsite finished: 847 snapshots OK",
     {"datastore": "offsite"}, dict(days=1)),
    ("offsite-pbs", "kernel", "info", "wg0: peer vyos handshake completed", {"interface": "wg0"}, dict(minutes=2)),

    ("htpc", "Windows Update", "info", "Installation finished: 2026-11 cumulative update for Windows 11 24H2", None, dict(days=2)),
    ("htpc", "Service Control Manager", "info", "The LabTether Agent service entered the running state", None, dict(days=6)),

    ("studio", "softwareupdated", "info", "No new software available", None, dict(hours=9)),
    ("studio", "backupd", "info", "Time Machine backup to nas01 finished: 3.1 GB copied", None, dict(hours=2)),
]

STORM_CONTAINERS = ["traefik", "paperless-web", "paperless-worker", "paperless-broker", "vaultwarden",
                    "authelia", "gitea", "uptime-kuma"]
STORM_LINES = [
    ("error", "Pull of image for container {c} failed: toomanyrequests: rate limit exceeded"),
    ("warning", "Container {c} exited with code 1, restarting (attempt {n})"),
    ("error", "Container {c} health check timed out after 30 seconds"),
    ("warning", "Container {c}: image not found locally, pulling"),
]
OUTAGE_HOSTS = ["pve01", "pve02", "nas01", "pbs01", "docker01", "k3s-cp1", "media01", "monitor01", "homeassistant"]


def log_events() -> list:
    rng = random.Random(42)
    events = list(RECENT)
    # The container restart loop on docker01 three days ago.
    for i in range(24):
        level, template = rng.choice(STORM_LINES)
        container = rng.choice(STORM_CONTAINERS)
        events.append(("docker01", "dockerd", level, template.format(c=container, n=rng.randint(2, 9)),
                       {"container": container}, dict(days=3, seconds=-i * 20)))
    events.append(("docker01", "dockerd", "info", "All 14 containers are running", None, dict(days=3, hours=-3)))
    # The network outage after the VyOS image update five days ago.
    for host in OUTAGE_HOSTS:
        events.append((host, "labtether-agent", "error", "Connection to the hub lost, reconnecting", None,
                       dict(days=5, minutes=-2, seconds=-rng.randint(0, 40))))
        events.append((host, "labtether-agent", "info", "Connected to the hub", None,
                       dict(days=5, minutes=-45, seconds=-rng.randint(0, 60))))
    events.append(("vyos", "kernel", "error", "r8169 0000:02:00.0 eth1: link down after driver reset", {"interface": "eth1"},
                   dict(days=5, minutes=-2)))
    events.append(("vyos", "systemd", "info", "Booted image 1.4.0 from the previous image list", None, dict(days=5, minutes=-44)))
    return events


def check_logs() -> None:
    for asset, _source, level, _message, fields, _when in log_events():
        check_asset(asset, "log line")
        if level not in {"info", "warning", "error", "debug"}:
            raise ValueError(f"log line: unknown level {level!r}")
        for key, value in (fields or {}).items():
            if not isinstance(value, str):
                raise ValueError(f"log line: field {key} on {asset} must be a string")


def logs_sql() -> str:
    rows = [
        [uid(f"log:{index}"), asset, source, level, message, fields, ago(**when)]
        for index, (asset, source, level, message, fields, when) in enumerate(log_events())
    ]
    return section("LOG EVENTS", insert(
        "log_events", ["id", "asset_id", "source", "level", "message", "fields", "timestamp"], rows))
