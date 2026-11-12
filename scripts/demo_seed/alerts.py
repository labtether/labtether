"""Alert rules, the alerts they raised, one silence, and where alerts were sent.

The hub's alert evaluator runs in the demo, so the rules use the conditions of
the hub's own starter rules and the open alerts are the ones the metrics and
heartbeats really produce: nas01's disk and the offline workstation.
"""

import hashlib

from .fleet import check_asset
from .sqlgen import DEMO_USER, ago, ahead, insert, section

# (id, name, description, kind, severity, window seconds, condition, labels)
RULES = [
    ("rule-device-offline", "Device offline", "No heartbeat from a device for 5 minutes.",
     "heartbeat_stale", "critical", 300, {"max_stale_seconds": 300}, {"signal": "offline"}),
    ("rule-disk-full", "Disk nearly full", "A disk at 90% or more.",
     "metric_threshold", "critical", 600,
     {"metric": "disk_used_percent", "operator": ">=", "value": 90, "aggregate": "max"}, {"signal": "disk"}),
    ("rule-high-cpu", "High CPU", "CPU at 90% or more for 5 minutes.",
     "metric_threshold", "high", 300,
     {"metric": "cpu_used_percent", "operator": ">=", "value": 90, "aggregate": "avg"}, {"signal": "cpu"}),
    ("rule-high-memory", "High memory", "Memory at 90% or more for 5 minutes.",
     "metric_threshold", "high", 300,
     {"metric": "memory_used_percent", "operator": ">=", "value": 90, "aggregate": "avg"}, {"signal": "memory"}),
    ("rule-high-temperature", "High temperature", "Temperature at 80 °C or more for 5 minutes.",
     "metric_threshold", "medium", 300,
     {"metric": "temperature_celsius", "operator": ">=", "value": 80, "aggregate": "avg"}, {"signal": "temperature"}),
    ("rule-error-logs", "Error log lines", "10 or more ERROR, FATAL or panic log lines in 5 minutes.",
     "log_pattern", "medium", 300, {"pattern": "ERROR|FATAL|panic", "min_occurrences": 10}, {"signal": "errors"}),
]

RULE_BY_ID = {rule[0]: rule for rule in RULES}


def fingerprint(rule_id: str, labels: dict) -> str:
    """The hub's alert fingerprint (internal/alerts GenerateFingerprint)."""
    digest = hashlib.sha256()
    digest.update(rule_id.encode() + b"\0")
    for key in sorted(labels):
        digest.update(key.encode() + b"\0" + labels[key].encode() + b"\0")
    return digest.hexdigest()


# (id, rule, status, devices, started, resolved or None, last fired)
# One open alert per rule at most: the hub keeps a single open alert per rule
# and lists the devices it fired for.
INSTANCES = [
    ("alert-nas01-disk", "rule-disk-full", "firing", ["nas01"], dict(hours=18), None, dict(seconds=30)),
    ("alert-workstation-offline", "rule-device-offline", "firing", ["workstation"],
     dict(hours=2, minutes=55), None, dict(seconds=30)),
    ("alert-docker01-cpu", "rule-high-cpu", "resolved", ["docker01"],
     dict(days=3, minutes=-4), dict(days=3, minutes=-58), dict(days=3, minutes=-57)),
    ("alert-vyos-offline", "rule-device-offline", "resolved", ["vyos"],
     dict(days=5, minutes=-7), dict(days=5, minutes=-46), dict(days=5, minutes=-45)),
    ("alert-media01-memory", "rule-high-memory", "resolved", ["media01"],
     dict(days=9, minutes=-10), dict(days=9, minutes=-70), dict(days=9, minutes=-69)),
    ("alert-pve01-temperature", "rule-high-temperature", "resolved", ["pve01"],
     dict(days=12, minutes=-6), dict(days=12, minutes=-31), dict(days=12, minutes=-30)),
]

INSTANCE_BY_ID = {instance[0]: instance for instance in INSTANCES}

# The CI runner pegs its CPU during builds, so High CPU is silenced on it.
SILENCES = [
    ("silence-ci-runner-cpu", {"rule_id": "rule-high-cpu", "asset_id": "ci-runner"},
     "Builds run at full CPU on this machine", dict(days=4), dict(days=26)),
]

# The channel is off: a demo hub must not send anything. Its history shows
# what a home lab's phone notifications look like.
NTFY_CHANNEL = "channel-ntfy-phone"
CHANNELS = [
    (NTFY_CHANNEL, "Phone (ntfy)", "ntfy",
     {"server_url": "https://ntfy.lab.example", "topic": "homelab-alerts"}, False, True),
]


def check_alerts() -> None:
    open_rules = set()
    for alert_id, rule_id, status, devices, _started, resolved, _fired in INSTANCES:
        if rule_id not in RULE_BY_ID:
            raise ValueError(f"alert {alert_id}: unknown rule {rule_id!r}")
        for device in devices:
            check_asset(device, f"alert {alert_id}")
        if status == "resolved":
            if resolved is None:
                raise ValueError(f"alert {alert_id}: resolved needs a time")
            continue
        if rule_id in open_rules:
            raise ValueError(f"alert {alert_id}: rule {rule_id} already has an open alert")
        open_rules.add(rule_id)
    for silence_id, matchers, _reason, _start, _end in SILENCES:
        if matchers.get("rule_id") not in RULE_BY_ID:
            raise ValueError(f"silence {silence_id}: unknown rule")
        check_asset(matchers["asset_id"], f"silence {silence_id}")


def _notification_rows() -> list:
    rows = []
    for alert_id, rule_id, _status, devices, started, resolved, _fired in INSTANCES:
        rule = RULE_BY_ID[rule_id]
        on = devices[0] if len(devices) == 1 else f"{len(devices)} devices"
        for state, when in (("firing", started), ("resolved", resolved)):
            if when is None:
                continue
            sent = ago(**when)
            payload = {
                "event": f"alert.{state}",
                "state": state,
                "alert_instance_id": alert_id,
                "rule_id": rule_id,
                "rule_name": rule[1],
                "severity": rule[4],
                "device_summary": on,
                "title": f"[{rule[4].upper()}] {rule[1]} on {on} ({state})",
            }
            rows.append([f"notification-{alert_id}-{state}", NTFY_CHANNEL, alert_id, None, "sent", sent, "",
                         sent, 0, 3, None, payload])
    return rows


def alerts_sql() -> str:
    created = ago(days=120)
    rule_rows = [
        [rule_id, name, description, "active", kind, severity, "global", 300, 120, 30, window, condition,
         labels, {"category": "starter"}, DEMO_USER, created, created]
        for rule_id, name, description, kind, severity, window, condition, labels in RULES
    ]
    instance_rows = []
    for alert_id, rule_id, status, devices, started, resolved, fired in INSTANCES:
        rule = RULE_BY_ID[rule_id]
        began = ago(**started)
        ended = ago(**resolved) if resolved else None
        instance_rows.append([
            alert_id, rule_id, fingerprint(rule_id, rule[7]), status, rule[4], rule[7], {}, began, ended,
            ago(**fired), None, began, ended or ago(**fired), began, {"version": 1, "asset_ids": devices},
        ])
    silence_rows = [
        [silence_id, matchers, reason, DEMO_USER, ago(**start), ahead(**end), ago(**start)]
        for silence_id, matchers, reason, start, end in SILENCES
    ]
    channel_rows = [
        [channel_id, name, kind, config, enabled, all_alerts, created, created]
        for channel_id, name, kind, config, enabled, all_alerts in CHANNELS
    ]
    return section(
        "ALERTS",
        insert("alert_rules",
               ["id", "name", "description", "status", "kind", "severity", "target_scope", "cooldown_seconds",
                "reopen_after_seconds", "evaluation_interval_seconds", "window_seconds", "condition", "labels",
                "metadata", "created_by", "created_at", "updated_at"],
               rule_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("alert_instances",
               ["id", "rule_id", "fingerprint", "status", "severity", "labels", "annotations", "started_at",
                "resolved_at", "last_fired_at", "suppressed_by", "created_at", "updated_at", "first_fired_at",
                "firing_scope"],
               instance_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("alert_silences",
               ["id", "matchers", "reason", "created_by", "starts_at", "ends_at", "created_at"],
               silence_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("notification_channels",
               ["id", "name", "type", "config", "enabled", "all_alerts", "created_at", "updated_at"],
               channel_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("notification_history",
               ["id", "channel_id", "alert_instance_id", "route_id", "status", "sent_at", "error", "created_at",
                "retry_count", "max_retries", "next_retry_at", "payload"],
               _notification_rows(), "ON CONFLICT (id) DO NOTHING"),
    )

