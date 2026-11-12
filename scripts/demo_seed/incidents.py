"""Issues: two open ones for the open alerts, one under investigation, two resolved."""

from .alerts import INSTANCE_BY_ID, RULE_BY_ID
from .fleet import check_asset, check_group
from .sqlgen import DEMO_USER, ago, insert, section

SYSTEM = "system"

# The hub opens an issue by itself for a critical alert five minutes after it
# fires, titled "<rule> on <device>". The two open alerts carry such an issue
# here, linked to the alert, so the hub does not open a second one.
INCIDENTS = [
    {
        "id": "incident-nas01-disk",
        "title": "Disk nearly full on nas01",
        "summary": "Pool tank on nas01 passed 90% used 18 hours ago and is at 91%. Snapshot retention on "
                   "tank/vms and tank/backups was cut from 30 to 14 days, which freed 2.1 TB. Two 16 TB "
                   "disks are ordered to widen the pool.",
        "status": "open", "severity": "critical", "source": "alert_auto",
        "group": "grp-storage", "asset": "nas01", "created_by": SYSTEM,
        "opened": dict(hours=17, minutes=55),
        "events": [
            ("alert_fired", "alert:alert-nas01-disk", "Disk nearly full fired on nas01", "critical", dict(hours=18)),
            ("audit", "note:scrub", "Scrub of tank started to rule out pool errors.", "info", dict(hours=16)),
            ("audit", "note:snapshots", "Snapshots hold 8.2 TB across tank/vms and tank/backups.", "info", dict(hours=14)),
            ("config_change", "config:retention", "Snapshot retention on tank/vms and tank/backups cut from 30 to 14 days.", "info", dict(hours=12)),
            ("metric_anomaly", "metric:rebound", "Disk use on nas01 back at 91% after the nightly backups.", "high", dict(hours=6)),
            ("audit", "note:disks", "Two 16 TB disks ordered. Delivery in 3 days.", "info", dict(hours=1)),
        ],
        "links": [("rule-disk-full", "alert-nas01-disk", "trigger")],
        "assets": [("nas01", "primary"), ("pve01", "impacted"), ("pve02", "impacted"), ("pbs01", "impacted")],
    },
    {
        "id": "incident-workstation-offline",
        "title": "Device offline on workstation",
        "summary": "workstation stopped reporting 3 hours ago. Its last log lines show NVMe timeouts and a "
                   "suspend. CI jobs that used its runner now wait for ci-runner.",
        "status": "open", "severity": "critical", "source": "alert_auto",
        "group": "grp-desks", "asset": "workstation", "created_by": SYSTEM,
        "opened": dict(hours=2, minutes=50),
        "events": [
            ("heartbeat_change", "heartbeat:workstation", "workstation stopped reporting.", "high", dict(hours=3)),
            ("alert_fired", "alert:alert-workstation-offline", "Device offline fired on workstation", "critical",
             dict(hours=2, minutes=55)),
            ("audit", "note:ssh", "SSH to workstation times out. Wake-on-LAN sent, no answer.", "info", dict(hours=1)),
        ],
        "links": [("rule-device-offline", "alert-workstation-offline", "trigger")],
        "assets": [("workstation", "primary"), ("ci-runner", "impacted")],
    },
    {
        "id": "incident-k3s-cert-manager",
        "title": "cert-manager webhook crash loop on k3s-cp1",
        "summary": "The cert-manager webhook pod has been in CrashLoopBackOff since the Helm upgrade to chart "
                   "1.16.2. Certificate renewals for ingress are blocked until it runs again.",
        "status": "investigating", "severity": "medium", "source": "manual",
        "group": "grp-services", "asset": "k3s-cp1", "created_by": DEMO_USER,
        "opened": dict(hours=1, minutes=50),
        "events": [
            ("config_change", "config:helm-upgrade", "cert-manager upgraded to chart 1.16.2 with Helm.", "info", dict(hours=2, minutes=3)),
            ("log_burst", "logs:cert-manager", "18 error lines from the cert-manager namespace in 10 minutes.", "high", dict(hours=1, minutes=55)),
            ("audit", "note:tls", "The webhook pod fails its TLS handshake with the API server.", "info", dict(hours=1, minutes=30)),
            ("action_run", "action:rollout-restart", "Command failed on k3s-cp1", "high", dict(hours=1)),
            ("audit", "note:rollback", "Next step: helm rollback cert-manager to chart 1.15.4.", "info", dict(minutes=30)),
        ],
        "links": [],
        "assets": [("k3s-cp1", "primary"), ("k3s-w1", "related")],
    },
    {
        "id": "incident-docker01-restart-loop",
        "title": "Container restart loop on docker01",
        "summary": "A scheduled image update hit the registry's pull rate limit. 9 of 13 containers on "
                   "docker01 restarted in a loop for about an hour.",
        "status": "resolved", "severity": "high", "source": "manual",
        "group": "grp-services", "asset": "docker01", "created_by": DEMO_USER,
        "opened": dict(days=3, minutes=-10),
        "mitigated": dict(days=3, hours=-1),
        "resolved": dict(days=3, hours=-3),
        "root_cause": "The weekly image update pulled every image at once and the registry answered 429. "
                      "Compose restart policies then restarted each container that had no image.",
        "action_items": ["Pin images by digest in every stack", "Run a pull-through registry cache on docker01",
                         "Update one stack at a time"],
        "lessons_learned": "Image updates need a pause between stacks.",
        "events": [
            ("alert_fired", "alert:alert-docker01-cpu", "High CPU fired on docker01", "high", dict(days=3, minutes=-4)),
            ("log_burst", "logs:dockerd", "24 warning and error lines from dockerd in 8 minutes.", "high", dict(days=3, minutes=-8)),
            ("audit", "note:rate-limit", "The registry answers 429 Too Many Requests to image pulls.", "info", dict(days=3, minutes=-25)),
            ("config_change", "config:compose-down", "Affected stacks stopped with docker compose down.", "info", dict(days=3, minutes=-30)),
            ("config_change", "config:pin-digests", "Images pinned by digest in all five stacks.", "info", dict(days=3, hours=-1)),
            ("action_run", "action:compose-up", "docker compose up -d: 13 containers running.", "info", dict(days=3, hours=-2)),
            ("alert_resolved", "alert:alert-docker01-cpu:resolved", "High CPU resolved on docker01", "info", dict(days=3, minutes=-58)),
        ],
        "links": [("rule-high-cpu", "alert-docker01-cpu", "related")],
        "assets": [("docker01", "primary"), ("pve01", "related")],
    },
    {
        "id": "incident-vyos-image-update",
        "title": "Network down after VyOS image update",
        "summary": "The LAN was down for 45 minutes after vyos rebooted into image 1.4.1. The network came "
                   "back after booting the previous image from the console.",
        "status": "resolved", "severity": "critical", "source": "manual",
        "group": "grp-network", "asset": "vyos", "created_by": DEMO_USER,
        "opened": dict(days=5),
        "mitigated": dict(days=5, minutes=-45),
        "resolved": dict(days=5, hours=-2),
        "root_cause": "The r8169 network driver in image 1.4.1 fails on the LAN port's Realtek chip.",
        "action_items": ["Hold vyos on image 1.4.0", "Add the serial console steps to the update checklist",
                         "Schedule router updates inside a maintenance window"],
        "lessons_learned": "Keep a console path to the router that does not need the LAN.",
        "events": [
            ("config_change", "config:image-update", "vyos image updated from 1.4.0 to 1.4.1 and rebooted.", "info", dict(days=5)),
            ("heartbeat_change", "heartbeat:vyos", "vyos stopped reporting.", "critical", dict(days=5, minutes=-2)),
            ("alert_fired", "alert:alert-vyos-offline", "Device offline fired on vyos", "critical", dict(days=5, minutes=-7)),
            ("log_burst", "logs:agents", "9 devices lost their connection to the hub within a minute.", "critical", dict(days=5, minutes=-8)),
            ("audit", "note:console", "Serial console shows the LAN link down after a driver reset.", "info", dict(days=5, minutes=-15)),
            ("config_change", "config:previous-image", "vyos booted from the previous image, 1.4.0.", "info", dict(days=5, minutes=-44)),
            ("alert_resolved", "alert:alert-vyos-offline:resolved", "Device offline resolved on vyos", "info", dict(days=5, minutes=-46)),
        ],
        "links": [("rule-device-offline", "alert-vyos-offline", "trigger")],
        "assets": [("vyos", "primary"), ("pihole", "impacted"), ("docker01", "impacted"), ("nas01", "impacted")],
    },
]


def check_incidents() -> None:
    for incident in INCIDENTS:
        where = f"issue {incident['id']}"
        check_asset(incident["asset"], where)
        check_group(incident["group"], where)
        refs = [event[1] for event in incident["events"]]
        if len(refs) != len(set(refs)):
            raise ValueError(f"{where}: timeline references must be unique")
        primaries = [asset for asset, role in incident["assets"] if role == "primary"]
        if primaries != [incident["asset"]]:
            raise ValueError(f"{where}: the primary device must be listed once")
        for asset, _role in incident["assets"]:
            check_asset(asset, where)
        for rule_id, alert_id, _kind in incident["links"]:
            if rule_id not in RULE_BY_ID or alert_id not in INSTANCE_BY_ID:
                raise ValueError(f"{where}: unknown alert link {rule_id} / {alert_id}")
            if INSTANCE_BY_ID[alert_id][1] != rule_id:
                raise ValueError(f"{where}: alert {alert_id} is not from rule {rule_id}")


def _time(incident: dict, key: str):
    return ago(**incident[key]) if incident.get(key) else None


def incidents_sql() -> str:
    incident_rows, event_rows, link_rows, asset_rows = [], [], [], []
    for incident in INCIDENTS:
        opened = ago(**incident["opened"])
        updated = _time(incident, "resolved") or ago(**incident["events"][-1][4])
        incident_rows.append([
            incident["id"], incident["title"], incident["summary"], incident["status"], incident["severity"],
            incident["source"], incident["group"], incident["asset"], None, incident["created_by"], opened,
            _time(incident, "mitigated"), _time(incident, "resolved"), None, incident.get("root_cause", ""),
            incident.get("action_items", []), incident.get("lessons_learned", ""), {}, opened, updated,
        ])
        for kind, ref, summary, severity, when in incident["events"]:
            at = ago(**when)
            event_rows.append([f"{incident['id']}:{ref}", incident["id"], kind, ref, summary, severity, {}, at, at])
        for rule_id, alert_id, link_type in incident["links"]:
            link_rows.append([f"{incident['id']}:{alert_id}", incident["id"], rule_id, alert_id,
                              None, link_type, incident["created_by"], opened])
        for asset, role in incident["assets"]:
            asset_rows.append([f"{incident['id']}:{asset}", incident["id"], asset, role, opened])
    return section(
        "ISSUES",
        insert("incidents",
               ["id", "title", "summary", "status", "severity", "source", "group_id", "primary_asset_id",
                "assignee", "created_by", "opened_at", "mitigated_at", "resolved_at", "closed_at", "root_cause",
                "action_items", "lessons_learned", "metadata", "created_at", "updated_at"],
               incident_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("incident_events",
               ["id", "incident_id", "event_type", "source_ref", "summary", "severity", "metadata",
                "occurred_at", "created_at"],
               event_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("incident_alert_links",
               ["id", "incident_id", "alert_rule_id", "alert_instance_id", "alert_fingerprint", "link_type",
                "created_by", "created_at"],
               link_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("incident_assets", ["id", "incident_id", "asset_id", "role", "created_at"],
               asset_rows, "ON CONFLICT (id) DO NOTHING"),
    )
