"""The demo visitor's account and two weeks of what its owner did in the console.

A viewer reads only the audit rows it caused, so every row belongs to the demo
user. The types are ones the 2.0 hub writes and the console has a sentence for.
"""

from .alerts import INSTANCE_BY_ID, RULE_BY_ID
from .fleet import check_asset
from .sqlgen import DEMO_USER, ago, insert, section, uid

# bcrypt hash of the demo password the hub itself uses (cmd/labtether/demo_bootstrap.go).
# The hub creates this user at startup when it is missing, so the row below
# only matters when the seed loads into a database the hub has not started on.
DEMO_PASSWORD_HASH = "$2a$12$rn/ay.JS7S08Ny8QE/tcqOOKhpjqvULRJR4TOqEZZk1NeeZQE/nPq"


def _alert(alert_id: str) -> dict:
    _id, rule_id, _status, devices, _started, _resolved, _fired = INSTANCE_BY_ID[alert_id]
    rule = RULE_BY_ID[rule_id]
    return {"target_name": rule[1], "rule_id": rule_id, "severity": rule[4], "device_id": devices[0],
            "device_name": devices[0], "device_count": 1}


# (type, target, decision, reason, details, how long ago)
EVENTS = [
    ("updates.run.completed", "update-run-backup-dry", "applied", "",
     {"target_name": "Backup hosts", "summary": "2 devices checked"}, dict(days=13)),
    ("alert.acknowledged", "alert-pve01-temperature", "applied", "", _alert("alert-pve01-temperature"), dict(days=12, minutes=-9)),
    ("terminal.session.created", "pve01", "allowed", "", {"mode": "ssh"}, dict(days=12, minutes=-11)),
    ("asset.updated", "studio", "applied", "",
     {"changed": ["group"], "group_id": "grp-desks", "group_name": "Desks", "previous_group_name": ""}, dict(days=11)),
    ("asset.updated", "vyos", "applied", "", {"changed": ["tags"], "tags": ["router", "firewall", "vpn"]}, dict(days=11, minutes=-4)),
    ("schedule.updated", "schedule-cert-check", "applied", "", {"target_name": "Check certificate expiry"}, dict(days=10)),
    ("alert.acknowledged", "alert-media01-memory", "applied", "", _alert("alert-media01-memory"), dict(days=9, minutes=-14)),
    ("updates.run.completed", "update-run-servers-previous", "applied", "",
     {"target_name": "Weekly patches: servers", "summary": "7 devices updated"}, dict(days=9, hours=3, minutes=41)),
    ("maintenance_window.created", "window-compute-kernel", "applied", "",
     {"target_name": "Kernel update on the Proxmox nodes", "group_name": "Compute"}, dict(days=8)),
    ("saved_action.run.completed", "saved-docker-prune", "applied", "", {"target_name": "Prune Docker images"}, dict(days=7)),
    ("terminal.session.created", "pve01", "allowed", "", {"mode": "ssh"}, dict(days=6, hours=2)),
    ("terminal.session.created", "pve02", "allowed", "", {"mode": "ssh"}, dict(days=6, hours=1, minutes=20)),
    ("updates.run.completed", "update-run-hypervisors-dry", "applied", "",
     {"target_name": "Proxmox nodes", "summary": "2 devices checked"}, dict(days=6, minutes=58)),
    ("incident.created", "incident-vyos-image-update", "applied", "",
     {"target_name": "Network down after VyOS image update", "severity": "critical"}, dict(days=5, minutes=-9)),
    ("incident.updated", "incident-vyos-image-update", "applied", "",
     {"target_name": "Network down after VyOS image update", "changed": ["status"], "status": "resolved"},
     dict(days=5, hours=-2)),
    ("alert.silence.created", "silence-ci-runner-cpu", "applied", "Builds run at full CPU on this machine",
     {"target_name": "High CPU on ci-runner", "matchers": {"rule_id": "rule-high-cpu", "asset_id": "ci-runner"}},
     dict(days=4)),
    ("incident.created", "incident-docker01-restart-loop", "applied", "",
     {"target_name": "Container restart loop on docker01", "severity": "high"}, dict(days=3, minutes=-10)),
    ("terminal.session.created", "docker01", "allowed", "", {"mode": "ssh"}, dict(days=3, minutes=-14)),
    ("actions.run.completed", "docker01", "applied", "", {"command": "docker compose up -d", "has_error": False},
     dict(days=3, hours=-2)),
    ("incident.updated", "incident-docker01-restart-loop", "applied", "",
     {"target_name": "Container restart loop on docker01", "changed": ["status"], "status": "resolved"},
     dict(days=3, hours=-3)),
    ("updates.run.completed", "update-run-servers-latest", "failed", "1 of 7 devices failed",
     {"target_name": "Weekly patches: servers", "summary": "6 of 7 devices updated"}, dict(days=2, hours=3, minutes=37)),
    ("actions.run.completed", "homeassistant", "applied", "", {"command": "ha core restart", "has_error": False}, dict(days=2)),
    ("webhook.updated", "webhook-inventory", "applied", "", {"target_name": "Inventory sheet sync", "enabled": False},
     dict(days=1, hours=5)),
    ("actions.run.completed", "docker01", "applied", "", {"command": "docker restart traefik", "has_error": False},
     dict(days=1, hours=2)),
    ("saved_action.created", "saved-backup-check", "allowed", "", {"target_name": "Last night's backups"}, dict(hours=20)),
    ("schedule.updated", "schedule-workstation-backup", "applied", "",
     {"target_name": "Back up home folders", "enabled": False}, dict(hours=2, minutes=40)),
    ("terminal.session.created", "nas01", "allowed", "", {"mode": "ssh"}, dict(hours=2, minutes=4)),
    ("actions.run.completed", "nas01", "applied", "", {"command": "zpool status -x", "has_error": False}, dict(hours=2)),
    ("incident.created", "incident-k3s-cert-manager", "applied", "",
     {"target_name": "cert-manager webhook crash loop on k3s-cp1", "severity": "medium"}, dict(hours=1, minutes=50)),
    ("actions.run.completed", "k3s-cp1", "failed", "pods are not ready after 60s",
     {"command": "kubectl rollout restart deployment/cert-manager-webhook -n cert-manager", "has_error": True},
     dict(hours=1, minutes=-1)),
    ("maintenance_window.created", "window-storage-disks", "applied", "",
     {"target_name": "Add two disks to tank on nas01", "group_name": "Storage"}, dict(minutes=50)),
    ("actions.run.completed", "docker01", "applied", "",
     {"command": "docker logs --tail 3 paperless-worker", "has_error": False}, dict(minutes=5)),
]

# Types whose target is a device.
DEVICE_TYPES = {"terminal.session.created", "asset.updated", "actions.run.completed"}


def check_audit() -> None:
    for kind, target, _decision, _reason, _details, _when in EVENTS:
        if kind in DEVICE_TYPES:
            check_asset(target, f"audit {kind}")


def demo_user_sql() -> str:
    created = ago(days=320)
    return section("DEMO USER", insert(
        "users", ["id", "username", "password_hash", "role", "auth_provider", "created_at", "updated_at"],
        [["usr-demo", "demo", DEMO_PASSWORD_HASH, "viewer", "local", created, created]],
        "ON CONFLICT (username) DO NOTHING"))


def audit_sql() -> str:
    rows = [
        [uid(f"audit:{index}"), kind, DEMO_USER, target, "", "", decision, reason, details, ago(**when)]
        for index, (kind, target, decision, reason, details, when) in enumerate(EVENTS)
    ]
    return section("AUDIT EVENTS", insert(
        "audit_events",
        ["id", "type", "actor_id", "target", "session_id", "command_id", "decision", "reason", "details", "timestamp"],
        rows))
