"""Day-to-day operations: saved actions, command runs, schedules, webhooks, updates and downtime windows."""

from .fleet import check_asset, check_group
from .sqlgen import DEMO_USER, Raw, ago, ahead, insert, section

# (id, name, description, [(step name, command, target)])
SAVED_ACTIONS = [
    ("saved-zfs-health", "ZFS pool health", "Pool state and space on nas01.",
     [("Pool status", "zpool status -x", "nas01"),
      ("Space by dataset", "zfs list -o name,used,avail -d 1 tank", "nas01")]),
    ("saved-docker-prune", "Prune Docker images", "Remove images no container uses.",
     [("Prune on docker01", "docker image prune -af", "docker01"),
      ("Prune on media01", "docker image prune -af", "media01"),
      ("Prune on monitor01", "docker image prune -af", "monitor01")]),
    ("saved-restart-proxy", "Restart reverse proxy", "Restart traefik and check it answers.",
     [("Restart traefik", "docker restart traefik", "docker01"),
      ("Check it answers", "curl -fsS -o /dev/null -w '%{http_code}\\n' http://127.0.0.1:80", "docker01")]),
    ("saved-pve-guests", "List Proxmox guests", "VMs and their state on both nodes.",
     [("Guests on pve01", "qm list", "pve01"), ("Guests on pve02", "qm list", "pve02")]),
    ("saved-k3s-pods", "Pods that are not running", "Every pod outside the Running state.",
     [("List pods", "kubectl get pods -A --field-selector=status.phase!=Running", "k3s-cp1")]),
    ("saved-pihole-gravity", "Update Pi-hole lists", "Fetch the blocklists again.",
     [("Update gravity", "pihole -g", "pihole")]),
    ("saved-backup-check", "Last night's backups", "Backup tasks on pbs01 and the offsite sync.",
     [("Tasks on pbs01", "proxmox-backup-manager task list --limit 5", "pbs01"),
      ("Sync on offsite-pbs", "proxmox-backup-manager task list --limit 3", "offsite-pbs")]),
]

# (target, command, status, output, error, how long ago, seconds it took)
ACTION_RUNS = [
    ("docker01", "docker ps --filter status=restarting --format '{{.Names}}'", "succeeded",
     "paperless-worker\n", "", dict(minutes=6), 2),
    ("docker01", "docker logs --tail 3 paperless-worker", "succeeded",
     "redis.exceptions.ConnectionError: connection refused, paperless-broker:6379\n", "", dict(minutes=5), 2),
    ("nas01", "zpool status -x", "succeeded", "all pools are healthy\n", "", dict(hours=2), 3),
    ("nas01", "zfs list -o name,used,avail -d 1 tank", "succeeded",
     "NAME          USED  AVAIL\ntank         87.4T  8.61T\ntank/backups 29.1T  8.61T\ntank/media   14.0T  8.61T\ntank/vms     44.3T  8.61T\n",
     "", dict(hours=1, minutes=58), 3),
    ("k3s-cp1", "kubectl rollout restart deployment/cert-manager-webhook -n cert-manager", "failed", "",
     "error: timed out waiting for the condition: pods are not ready after 60s", dict(hours=1), 65),
    ("k3s-cp1", "kubectl get pods -n cert-manager", "succeeded",
     "NAME                                       READY   STATUS             RESTARTS   AGE\n"
     "cert-manager-7f9b4c8d6-abc12               1/1     Running            0          2h\n"
     "cert-manager-cainjector-6c8d9b4-def34      1/1     Running            0          2h\n"
     "cert-manager-webhook-7f9b4c8d6-x2k4m       0/1     CrashLoopBackOff   12         2h\n",
     "", dict(minutes=50), 4),
    ("pve01", "qm list", "succeeded",
     "      VMID NAME                 STATUS     MEM(MB)    BOOTDISK(GB) PID\n"
     "       100 docker01             running    32768             200.00 2214\n"
     "       101 k3s-cp1              running    8192               60.00 2390\n"
     "       102 ci-runner            running    16384             160.00 2533\n"
     "       104 minecraft            running    12288              80.00 2710\n",
     "", dict(hours=3), 2),
    ("pbs01", "proxmox-backup-manager task list --limit 5", "succeeded",
     "backup vm/100   OK\nbackup vm/101   OK\nbackup vm/102   OK\nverify main     OK\ngarbage_collection main   OK\n",
     "", dict(hours=5), 3),
    ("pihole", "pihole -g", "succeeded",
     "  [i] Number of gravity domains: 152891 (152891 unique domains)\n  [i] Done.\n", "", dict(hours=12), 41),
    ("docker01", "docker restart traefik", "succeeded", "traefik\n", "", dict(days=1, hours=2), 4),
    ("offsite-pbs", "wg show wg0 latest-handshakes", "succeeded", "vyos\t12 seconds ago\n", "", dict(days=1, hours=4), 2),
    ("homeassistant", "ha core restart", "succeeded", "Command completed successfully.\n", "", dict(days=2), 34),
    ("pve02", "pvecm status | grep -E 'Nodes|Quorate'", "succeeded", "Nodes:            2\nQuorate:          Yes\n", "",
     dict(days=2, hours=3), 3),
]


def _next_run(base: str, offset: str, step: str) -> str:
    """The next time a repeating slot comes up, in UTC: base + offset, or one step later if that has passed."""
    slot = f"(date_trunc('{base}', NOW(), 'UTC') + INTERVAL '{offset}')"
    return f"(CASE WHEN {slot} > NOW() THEN {slot} ELSE {slot} + INTERVAL '{step}' END)"


def _daily(hour: int, minute: int = 0) -> tuple:
    return _next_run("day", f"{hour} hours {minute} minutes", "1 day"), "1 day"


def _weekly(weekday_after_monday: int, hour: int) -> tuple:
    return _next_run("week", f"{weekday_after_monday} days {hour} hours", "7 days"), "7 days"


def _hourly(minute: int) -> tuple:
    return _next_run("hour", f"{minute} minutes", "1 hour"), "1 hour"


# (id, name, cron, command, targets, group, enabled, (next run SQL, period), last status, last error)
# Next and last run are worked out at load time from the same cron slot (UTC).
SCHEDULES = [
    ("schedule-apt-security", "Security updates (Debian and Ubuntu)", "30 3 * * *",
     "unattended-upgrade", ["docker01", "media01", "monitor01", "k3s-cp1", "k3s-w1", "ci-runner", "minecraft"],
     None, True, _daily(3, 30), "succeeded", ""),
    ("schedule-docker-prune", "Prune Docker images", "0 4 * * 0", "docker image prune -af",
     ["docker01", "media01", "monitor01"], None, True, _weekly(6, 4), "succeeded", ""),
    ("schedule-zfs-snapshot-count", "Count ZFS snapshots", "15 * * * *", "zfs list -t snapshot -H | wc -l",
     ["nas01"], None, True, _hourly(15), "succeeded", ""),
    ("schedule-pihole-gravity", "Update Pi-hole lists", "0 5 * * 1", "pihole -g", ["pihole"], None, True,
     _weekly(0, 5), "succeeded", ""),
    ("schedule-cert-check", "Check certificate expiry", "0 6 * * *",
     "kubectl get certificates -A -o wide", ["k3s-cp1"], None, True, _daily(6), "failed",
     "exit status 1: Internal error occurred: failed calling webhook \"webhook.cert-manager.io\""),
    ("schedule-smart-short", "SMART short tests", "0 2 * * 6",
     "for d in /dev/sd?; do smartctl -t short \"$d\"; done", ["pve01", "pve02", "nas01", "pbs01"],
     "grp-homelab", True, _weekly(5, 2), "succeeded", ""),
    ("schedule-workstation-backup", "Back up home folders", "0 1 * * *",
     "restic backup /home --repo sftp:nas01:/mnt/tank/backups/workstation", ["workstation"], None, False,
     None, "skipped", ""),
]

# (id, name, url, events, enabled, last sent)
# The addresses use the reserved .example domain, which never resolves.
WEBHOOKS = [
    ("webhook-ntfy", "ntfy alerts", "https://ntfy.lab.example/homelab-alerts",
     ["alert.fired", "alert.resolved"], True, dict(hours=2, minutes=55)),
    ("webhook-home-assistant", "Home Assistant automations",
     "https://homeassistant.lab.example/api/webhook/labtether-events",
     ["alert.fired", "alert.resolved", "incident.created", "incident.resolved"], True, dict(hours=2, minutes=50)),
    ("webhook-chat-ops", "Chat room for the lab", "https://chat.lab.example/hooks/labtether",
     ["incident.created", "incident.resolved", "update.completed", "action.completed"], True, dict(minutes=50)),
    ("webhook-inventory", "Inventory sheet sync", "https://n8n.lab.example/webhook/device-inventory",
     ["asset.created", "asset.updated", "asset.deleted"], False, dict(days=45)),
]

LINUX_SERVERS = ["docker01", "media01", "monitor01", "k3s-cp1", "k3s-w1", "ci-runner", "minecraft"]

# (id, name, description, targets, dry run by default)
UPDATE_PLANS = [
    ("plan-weekly-servers", "Weekly patches: servers", "Debian and Ubuntu VMs, Sunday morning.", LINUX_SERVERS, False),
    ("plan-hypervisors", "Proxmox nodes", "One node at a time, guests migrated first.", ["pve01", "pve02"], True),
    ("plan-backup-hosts", "Backup hosts", "pbs01 and the offsite copy.", ["pbs01", "offsite-pbs"], True),
]

# (id, plan, dry run, status, summary, error, {target: (status, summary)}, how long ago, minutes it took)
UPDATE_RUNS = [
    ("update-run-servers-latest", "plan-weekly-servers", False, "failed", "6 of 7 devices updated", "",
     {"docker01": ("succeeded", "14 packages upgraded"), "media01": ("succeeded", "9 packages upgraded"),
      "monitor01": ("succeeded", "11 packages upgraded"), "k3s-cp1": ("succeeded", "6 packages upgraded"),
      "k3s-w1": ("succeeded", "6 packages upgraded"), "ci-runner": ("succeeded", "21 packages upgraded"),
      "minecraft": ("failed", "dpkg was interrupted, run 'dpkg --configure -a'")},
     dict(days=2, hours=4), 23),
    ("update-run-hypervisors-dry", "plan-hypervisors", True, "succeeded", "2 devices checked", "",
     {"pve01": ("succeeded", "38 packages can be upgraded"), "pve02": ("succeeded", "38 packages can be upgraded")},
     dict(days=6, hours=1), 2),
    ("update-run-servers-previous", "plan-weekly-servers", False, "succeeded", "7 devices updated", "",
     {target: ("succeeded", "packages upgraded") for target in LINUX_SERVERS}, dict(days=9, hours=4), 19),
    ("update-run-backup-dry", "plan-backup-hosts", True, "succeeded", "2 devices checked", "",
     {"pbs01": ("succeeded", "12 packages can be upgraded"), "offsite-pbs": ("succeeded", "17 packages can be upgraded")},
     dict(days=13), 1),
]

# (id, group, name, start, end, suppress alerts, block actions, block updates, created)
MAINTENANCE_WINDOWS = [
    ("window-compute-kernel", "grp-compute", "Kernel update on the Proxmox nodes",
     ago(days=6, hours=2), ago(days=6), True, False, False, ago(days=8)),
    ("window-storage-disks", "grp-storage", "Add two disks to tank on nas01",
     ahead(days=3), ahead(days=3, hours=4), True, True, True, ago(minutes=50)),
]


def check_operations() -> None:
    for action_id, _name, _description, steps in SAVED_ACTIONS:
        for _step, _command, target in steps:
            check_asset(target, f"saved action {action_id}")
    for run in ACTION_RUNS:
        check_asset(run[0], "action run")
        if run[2] not in {"succeeded", "failed"}:
            raise ValueError(f"action run on {run[0]}: unknown status {run[2]!r}")
    for schedule in SCHEDULES:
        for target in schedule[4]:
            check_asset(target, f"schedule {schedule[0]}")
        if schedule[5] is not None:
            check_group(schedule[5], f"schedule {schedule[0]}")
    plan_ids = set()
    for plan_id, _name, _description, targets, _dry in UPDATE_PLANS:
        plan_ids.add(plan_id)
        for target in targets:
            check_asset(target, f"update plan {plan_id}")
    for run_id, plan_id, _dry, _status, _summary, _error, results, _when, _minutes in UPDATE_RUNS:
        if plan_id not in plan_ids:
            raise ValueError(f"update run {run_id}: unknown plan {plan_id!r}")
        for target in results:
            check_asset(target, f"update run {run_id}")
    for window in MAINTENANCE_WINDOWS:
        check_group(window[1], f"maintenance window {window[0]}")


def _saved_action_rows() -> list:
    rows = []
    for index, (action_id, name, description, steps) in enumerate(SAVED_ACTIONS):
        step_list = [{"name": step, "command": command, "target": target} for step, command, target in steps]
        rows.append([action_id, name, description, step_list, DEMO_USER, ago(days=40 - index * 4)])
    return rows


def _action_run_rows() -> list:
    rows = []
    for index, (target, command, status, output, error, when, seconds) in enumerate(ACTION_RUNS):
        started = ago(**when)
        finished = Raw(f"{started.sql} + INTERVAL '{seconds} seconds'")
        rows.append([f"action-run-{index + 1:02d}", "command", DEMO_USER, target, command, None, None, None, False,
                     status, output, error, started, finished, finished])
    return rows


def _schedule_rows() -> list:
    rows = []
    for index, (schedule_id, name, cron, command, targets, group, enabled, slot, status, error) in enumerate(SCHEDULES):
        if slot is None:
            # Paused: no next run, and the last one was when its device was still on.
            next_run, last_run = None, ago(days=1, hours=6)
        else:
            next_sql, period = slot
            next_run, last_run = Raw(next_sql), Raw(f"{next_sql} - INTERVAL '{period}'")
        rows.append([schedule_id, name, cron, command, targets, group, enabled, DEMO_USER,
                     ago(days=90 - index * 7), last_run, next_run, status, error, None])
    return rows


def _update_run_rows() -> list:
    plan_names = {plan[0]: plan[1] for plan in UPDATE_PLANS}
    rows = []
    for run_id, plan_id, dry_run, status, summary, error, results, when, minutes in UPDATE_RUNS:
        started = ago(**when)
        finished = Raw(f"{started.sql} + INTERVAL '{minutes} minutes'")
        entries = [{"target": target, "scope": "os_packages", "status": state, "summary": text}
                   for target, (state, text) in results.items()]
        rows.append([run_id, plan_id, plan_names[plan_id], DEMO_USER, dry_run, status, summary, error, entries,
                     started, finished, finished])
    return rows


def operations_sql() -> str:
    webhook_rows = [
        [webhook_id, name, url, "", events, enabled, DEMO_USER, ago(days=75 - index * 9), ago(**last), ""]
        for index, (webhook_id, name, url, events, enabled, last) in enumerate(WEBHOOKS)
    ]
    plan_rows = [
        [plan_id, name, description, targets, ["os_packages"], dry_run, ago(days=60), ago(days=60)]
        for plan_id, name, description, targets, dry_run in UPDATE_PLANS
    ]
    window_rows = [
        [window_id, group, name, start, end, suppress, block_actions, block_updates, created, created]
        for window_id, group, name, start, end, suppress, block_actions, block_updates, created in MAINTENANCE_WINDOWS
    ]
    return section(
        "OPERATIONS",
        insert("saved_actions", ["id", "name", "description", "steps", "created_by", "created_at"],
               _saved_action_rows(), "ON CONFLICT (id) DO NOTHING"),
        insert("action_runs",
               ["id", "type", "actor_id", "target", "command", "connector_id", "action_id", "params", "dry_run",
                "status", "output", "error", "created_at", "updated_at", "completed_at"],
               _action_run_rows(), "ON CONFLICT (id) DO NOTHING"),
        insert("scheduled_tasks",
               ["id", "name", "cron_expr", "command", "targets", "group_id", "enabled", "created_by",
                "created_at", "last_run_at", "next_run_at", "last_run_status", "last_error", "last_run_job_id"],
               _schedule_rows(), "ON CONFLICT (id) DO NOTHING"),
        insert("webhooks",
               ["id", "name", "url", "secret", "events", "enabled", "created_by", "created_at",
                "last_triggered_at", "secret_ciphertext"],
               webhook_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("update_plans",
               ["id", "name", "description", "targets", "scopes", "default_dry_run", "created_at", "updated_at"],
               plan_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("update_runs",
               ["id", "plan_id", "plan_name", "actor_id", "dry_run", "status", "summary", "error", "results",
                "created_at", "updated_at", "completed_at"],
               _update_run_rows(), "ON CONFLICT (id) DO NOTHING"),
        insert("group_maintenance_windows",
               ["id", "group_id", "name", "start_at", "end_at", "suppress_alerts", "block_actions",
                "block_updates", "created_at", "updated_at"],
               window_rows, "ON CONFLICT (id) DO NOTHING"),
    )
