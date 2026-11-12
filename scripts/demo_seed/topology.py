"""How the hosts relate: links between them and the saved Map layout."""

from .fleet import HOSTS, check_asset
from .sqlgen import NOW, ago, insert, section, uid

ROUTER = "vyos"

# A VM runs on its hypervisor: (guest, host).
RUNS_ON = [
    ("docker01", "pve01"),
    ("k3s-cp1", "pve01"),
    ("ci-runner", "pve01"),
    ("minecraft", "pve01"),
    ("k3s-w1", "pve02"),
    ("monitor01", "pve02"),
    ("media01", "pve02"),
]

# (source, target, relationship, criticality, note)
OTHER_EDGES = [
    ("nas01", "pve01", "provides_to", "critical", "NFS datastore for VM disks"),
    ("nas01", "pve02", "provides_to", "critical", "NFS datastore for VM disks"),
    ("nas01", "pbs01", "provides_to", "high", "NFS share for the backup datastore"),
    ("pbs01", "offsite-pbs", "depends_on", "high", "Nightly sync over WireGuard"),
]

# Machines plugged into the home LAN reach everything else through the router.
# A VM reaches the LAN through its hypervisor's bridge.
GUESTS = {guest for guest, _host in RUNS_ON}
LAN_HOSTS = [host.id for host in HOSTS
             if host.ip.startswith("10.0.10.") and host.id != ROUTER and host.id not in GUESTS]


def edges() -> list:
    """Every link as (source, target, relationship, criticality, note)."""
    result = [(guest, host, "runs_on", "high", f"VM on {host}") for guest, host in RUNS_ON]
    result += OTHER_EDGES
    result += [(host, ROUTER, "connected_to", "medium", "") for host in LAN_HOSTS]
    result += [(guest, host, "connected_to", "medium", "") for guest, host in RUNS_ON]
    return result


TOPOLOGY_ID = uid("topology:home-lab")

# (key, label, colour, icon, x, y, width, height)
ZONES = [
    ("network", "Network", "blue", "globe", 0, 0, 500, 220),
    ("compute", "Compute", "purple", "cpu", 600, 0, 500, 220),
    ("storage", "Storage", "orange", "database", 1200, 0, 500, 220),
    ("services", "Services", "green", "layers", 0, 320, 1100, 380),
    ("desks", "Desks", "cyan", "code", 1200, 320, 500, 380),
    ("offsite", "Offsite", "gray", "radio", 1200, 800, 500, 220),
]

# zone key -> [(asset, x, y)] with positions inside the zone.
ZONE_MEMBERS = {
    "network": [("vyos", 60, 70), ("pihole", 280, 70)],
    "compute": [("pve01", 60, 70), ("pve02", 280, 70)],
    "storage": [("nas01", 60, 70), ("pbs01", 280, 70)],
    "services": [
        ("docker01", 60, 70), ("media01", 320, 70), ("monitor01", 580, 70), ("homeassistant", 840, 70),
        ("k3s-cp1", 60, 220), ("k3s-w1", 320, 220), ("minecraft", 580, 220),
    ],
    "desks": [("ci-runner", 60, 70), ("workstation", 280, 70), ("htpc", 60, 220), ("studio", 280, 220)],
    "offsite": [("offsite-pbs", 170, 70)],
}


def zone_id(key: str) -> str:
    return uid(f"zone:{key}")


def check_topology() -> None:
    seen = set()
    for source, target, _rel, _crit, _note in edges():
        check_asset(source, "edge")
        check_asset(target, "edge")
    for key, members in ZONE_MEMBERS.items():
        if key not in {zone[0] for zone in ZONES}:
            raise ValueError(f"zone members: unknown zone {key!r}")
        for asset, _x, _y in members:
            check_asset(asset, f"zone {key}")
            if asset in seen:
                raise ValueError(f"zone members: {asset} is in two zones")
            seen.add(asset)
    missing = {host.id for host in HOSTS} - seen
    if missing:
        raise ValueError(f"zone members: no zone for {sorted(missing)}")


def topology_sql() -> str:
    created = ago(days=120)
    edge_rows = [
        [uid(f"edge:{source}:{target}:{rel}"), source, target, rel, "downstream", crit, "manual", 1.0,
         None, {"note": note} if note else {}, created, created]
        for source, target, rel, crit, note in edges()
    ]
    zone_rows = [
        [zone_id(key), TOPOLOGY_ID, None, label, colour, icon, {"x": x, "y": y},
         {"width": width, "height": height}, order, NOW, NOW]
        for order, (key, label, colour, icon, x, y, width, height) in enumerate(ZONES)
    ]
    member_rows = [
        [zone_id(key), asset, {"x": x, "y": y}, order]
        for key, members in ZONE_MEMBERS.items()
        for order, (asset, x, y) in enumerate(members)
    ]
    connection_rows = [
        [uid(f"connection:{source}:{target}:{rel}"), TOPOLOGY_ID, source, target, rel, True, "", NOW]
        for source, target, rel, _crit, _note in edges()
    ]
    return section(
        "TOPOLOGY",
        insert("asset_edges",
               ["id", "source_asset_id", "target_asset_id", "relationship_type", "direction", "criticality",
                "origin", "confidence", "match_signals", "metadata", "created_at", "updated_at"],
               edge_rows, "ON CONFLICT DO NOTHING"),
        insert("topology_layouts", ["id", "name", "viewport", "created_at", "updated_at"],
               [[TOPOLOGY_ID, "Home lab", {"x": 60, "y": 40, "zoom": 0.7}, NOW, NOW]],
               "ON CONFLICT (id) DO NOTHING"),
        insert("topology_zones",
               ["id", "topology_id", "parent_zone_id", "label", "color", "icon", "position", "size",
                "sort_order", "created_at", "updated_at"],
               zone_rows, "ON CONFLICT (id) DO NOTHING"),
        insert("zone_members", ["zone_id", "asset_id", "position", "sort_order"],
               member_rows, "ON CONFLICT (zone_id, asset_id) DO NOTHING"),
        insert("topology_connections",
               ["id", "topology_id", "source_asset_id", "target_asset_id", "relationship", "user_defined",
                "label", "created_at"],
               connection_rows, "ON CONFLICT DO NOTHING"),
    )
