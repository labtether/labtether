"""Thirty days of metric history, built inside PostgreSQL when the seed loads.

The file carries one small profile row per host and metric. A single
INSERT ... SELECT expands them over generate_series, so the samples always end
at the load time and sit on the quarter hours the hub's live demo data thins
down to.
"""

from .fleet import HOSTS, OFFLINE_SINCE_HOURS, check_asset
from .sqlgen import header

HISTORY_DAYS = 30
STEP_SECONDS = 900
MBIT = 125000  # bytes per second in one Mbit/s

# Daily swing of CPU load as a share of the base load. Hosts people use in the
# evening swing more than appliances.
CPU_SWING = {
    "media01": 0.6, "minecraft": 1.2, "htpc": 1.5, "ci-runner": 0.9, "studio": 0.8,
    "workstation": 0.4, "vyos": 0.4, "pihole": 0.3, "offsite-pbs": 0.2, "pbs01": 0.5,
}
DEFAULT_CPU_SWING = 0.3

# How much each host's disk filled over the 30 days, in percentage points.
DISK_GROWTH = {"nas01": 7.0, "pbs01": 3.0, "monitor01": 2.5, "media01": 2.0, "offsite-pbs": 1.5}
DEFAULT_DISK_GROWTH = 0.4

# (asset, metric, hours ago, hours wide, amount): a bell-shaped bump in a series.
SPIKES = [
    # docker01: the container restart loop three days ago.
    ("docker01", "cpu_used_percent", 71.5, 1.5, 58.0),
    ("docker01", "memory_used_percent", 71.5, 1.5, 22.0),
    ("docker01", "temperature_celsius", 71.5, 1.5, 14.0),
    ("pve01", "cpu_used_percent", 71.5, 1.5, 24.0),
    # vyos: the image update five days ago.
    ("vyos", "cpu_used_percent", 119.8, 0.8, 55.0),
    # nas01: the pool scrub that ended eight hours ago.
    ("nas01", "cpu_used_percent", 11.0, 6.0, 18.0),
    ("nas01", "temperature_celsius", 11.0, 6.0, 6.0),
    # media01: a long transcode nine days ago.
    ("media01", "memory_used_percent", 216.0, 2.0, 16.0),
    ("media01", "cpu_used_percent", 216.0, 2.0, 45.0),
    # pve01: a fan fault twelve days ago.
    ("pve01", "temperature_celsius", 288.0, 1.2, 34.0),
    # pbs01 and offsite-pbs: last night's backup and sync.
    ("pbs01", "cpu_used_percent", 7.0, 1.5, 40.0),
    ("pbs01", "network_rx_bytes_per_sec", 7.0, 1.5, 480.0 * MBIT),
    ("offsite-pbs", "network_rx_bytes_per_sec", 5.0, 3.0, 28.0 * MBIT),
]


def profiles() -> list:
    """(asset, metric, unit, base, noise, day swing, growth, low, high, stop seconds)."""
    rows = []
    for host in HOSTS:
        stop = 0 if host.online else OFFLINE_SINCE_HOURS * 3600
        swing = CPU_SWING.get(host.id, DEFAULT_CPU_SWING)
        cpu_swing = round(host.cpu_base * swing, 2)
        cpu_noise = round(1.5 + host.cpu_base * 0.12, 2)
        temp_base = round(38 + host.cpu_base * 0.32, 2)
        rx = host.net_mbit * MBIT
        rows += [
            (host.id, "cpu_used_percent", "percent", host.cpu_base, cpu_noise, cpu_swing, 0.0, 1.0, 100.0, stop),
            (host.id, "memory_used_percent", "percent", host.mem_base, 1.5, 2.0, 1.0, 5.0, 100.0, stop),
            (host.id, "disk_used_percent", "percent", host.disk_pct, 0.15, 0.0,
             DISK_GROWTH.get(host.id, DEFAULT_DISK_GROWTH), 1.0, 100.0, stop),
            (host.id, "temperature_celsius", "celsius", temp_base, 1.0, round(cpu_swing * 0.3, 2), 0.0, 25.0, 95.0, stop),
            (host.id, "network_rx_bytes_per_sec", "bytes_per_sec", rx, round(rx * 0.35), round(rx * 0.5), 0.0,
             round(rx * 0.02), rx * 40, stop),
            (host.id, "network_tx_bytes_per_sec", "bytes_per_sec", round(rx * 0.3), round(rx * 0.12),
             round(rx * 0.15), 0.0, round(rx * 0.01), rx * 12, stop),
        ]
    return rows


def check_telemetry() -> None:
    known = {(row[0], row[1]) for row in profiles()}
    for asset, metric, _hours_ago, hours_wide, _amount in SPIKES:
        check_asset(asset, "metric spike")
        if (asset, metric) not in known:
            raise ValueError(f"metric spike: no profile for {asset} {metric}")
        if hours_wide <= 0:
            raise ValueError(f"metric spike: {asset} {metric} needs a width")


def _number(value) -> str:
    return f"{float(value):.2f}"


def metrics_sql() -> str:
    profile_rows = ",\n".join(
        "        ('{}', '{}', '{}', {}, {}, {}, {}, {}, {}, {})".format(
            asset, metric, unit, _number(base), _number(noise), _number(swing), _number(growth),
            _number(low), _number(high), int(stop))
        for asset, metric, unit, base, noise, swing, growth, low, high, stop in profiles()
    )
    spike_rows = ",\n".join(
        "        ('{}', '{}', {}, {}, {})".format(
            asset, metric, int(hours_ago * 3600), int(hours_wide * 3600), _number(amount))
        for asset, metric, hours_ago, hours_wide, amount in SPIKES
    )
    history_seconds = HISTORY_DAYS * 86400
    return header(f"METRIC SAMPLES ({HISTORY_DAYS} days, one sample every 15 minutes)") + f"""-- value = base
--       + day swing (highest at 14:00 UTC, lowest at 02:00)
--       - growth still to come (a series ends at its base)
--       + spikes (bell curves)
--       + noise (three uniform draws, close to a normal curve)
-- stop_seconds ends a series early: the offline workstation stopped reporting.
SELECT setseed(0.42);

WITH profile (asset_id, metric, unit, base, noise, swing, growth, low, high, stop_seconds) AS (
    VALUES
{profile_rows}
),
spike (asset_id, metric, center_seconds, width_seconds, amount) AS (
    VALUES
{spike_rows}
),
tick AS (
    SELECT g AS sampled_at,
           EXTRACT(EPOCH FROM (NOW() - g))::double precision AS age,
           ((EXTRACT(EPOCH FROM g)::bigint % 86400) / 3600.0)::double precision AS hour_utc
    FROM generate_series(
        to_timestamp((floor(EXTRACT(EPOCH FROM NOW()) / {STEP_SECONDS}) * {STEP_SECONDS} - {history_seconds})::double precision),
        to_timestamp((floor(EXTRACT(EPOCH FROM NOW()) / {STEP_SECONDS}) * {STEP_SECONDS})::double precision),
        INTERVAL '{STEP_SECONDS} seconds'
    ) AS g
)
INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at)
SELECT p.asset_id,
       p.metric,
       p.unit,
       round(LEAST(p.high::double precision, GREATEST(p.low::double precision,
           p.base::double precision
           + p.swing::double precision * sin((t.hour_utc - 8.0) * pi() / 12.0)
           - p.growth::double precision * (t.age / {history_seconds}.0)
           + COALESCE((
               SELECT sum(s.amount::double precision
                          * exp(-8.0 * power((s.center_seconds::double precision - t.age) / s.width_seconds::double precision, 2.0)))
               FROM spike s
               WHERE s.asset_id = p.asset_id AND s.metric = p.metric
             ), 0.0)
           + p.noise::double precision * (random() + random() + random() - 1.5) * 1.15
       ))::numeric, 2)::double precision,
       t.sampled_at
FROM profile p
CROSS JOIN tick t
WHERE t.age >= p.stop_seconds::double precision
ORDER BY p.asset_id, p.metric, t.sampled_at;

"""
