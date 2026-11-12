"""The demo home lab: groups and the agent hosts, with the facts a 2.0 agent reports."""

from dataclasses import dataclass, field

from .sqlgen import ago, insert, section

GIB = 1024 ** 3

# (id, name, slug, parent, icon, sort order)
GROUPS = [
    ("grp-homelab", "Home lab", "home-lab", None, "server", 1),
    ("grp-compute", "Compute", "compute", "grp-homelab", "cpu", 1),
    ("grp-storage", "Storage", "storage", "grp-homelab", "database", 2),
    ("grp-network", "Network", "network", "grp-homelab", "globe", 3),
    ("grp-services", "Services", "services", "grp-homelab", "layers", 4),
    ("grp-desks", "Desks", "desks", None, "code", 2),
    ("grp-offsite", "Offsite", "offsite", None, "radio", 3),
]

GROUP_IDS = {group[0] for group in GROUPS}


@dataclass(frozen=True)
class OS:
    """What the agent reads from the operating system."""

    family: str  # the agent's `os` value: linux, windows or darwin
    os_id: str
    name: str
    version: str
    pretty: str
    kernel: str


@dataclass(frozen=True)
class CPU:
    model: str
    vendor: str
    arch: str = "amd64"


PVE = OS("linux", "debian", "Debian GNU/Linux", "12 (bookworm)",
         "Proxmox VE 8.4", "6.8.12-11-pve")
TRUENAS = OS("linux", "debian", "Debian GNU/Linux", "12 (bookworm)",
             "TrueNAS SCALE 25.04", "6.12.15-production+truenas")
PBS = OS("linux", "debian", "Debian GNU/Linux", "12 (bookworm)",
         "Proxmox Backup Server 3.4", "6.8.12-11-pve")
VYOS = OS("linux", "vyos", "VyOS", "1.4.0", "VyOS 1.4.0 (sagitta)", "6.6.89-amd64-vyos")
RASPBERRY_PI_OS = OS("linux", "debian", "Debian GNU/Linux", "12 (bookworm)",
                     "Debian GNU/Linux 12 (bookworm)", "6.12.25+rpt-rpi-2712")
DEBIAN = OS("linux", "debian", "Debian GNU/Linux", "12 (bookworm)",
            "Debian GNU/Linux 12 (bookworm)", "6.1.0-37-amd64")
UBUNTU = OS("linux", "ubuntu", "Ubuntu", "24.04.3 LTS (Noble Numbat)",
            "Ubuntu 24.04.3 LTS", "6.8.0-71-generic")
HAOS = OS("linux", "haos", "Home Assistant OS", "16.2", "Home Assistant OS 16.2", "6.12.35-haos")
FEDORA = OS("linux", "fedora", "Fedora Linux", "42 (Workstation Edition)",
            "Fedora Linux 42 (Workstation Edition)", "6.15.9-201.fc42.x86_64")
WINDOWS = OS("windows", "windows", "Windows 11 Pro", "24H2", "Windows 11 Pro 24H2", "10.0.26100")
MACOS = OS("darwin", "macos", "macOS", "26.1", "macOS 26.1", "25.1.0")

RYZEN_5950X = CPU("AMD Ryzen 9 5950X 16-Core Processor", "AuthenticAMD")
CORE_I7_11700 = CPU("11th Gen Intel(R) Core(TM) i7-11700 @ 2.50GHz", "GenuineIntel")
CORE_I5_12400 = CPU("12th Gen Intel(R) Core(TM) i5-12400", "GenuineIntel")
CORE_I7_7700 = CPU("Intel(R) Core(TM) i7-7700 CPU @ 3.60GHz", "GenuineIntel")
INTEL_N100 = CPU("Intel(R) N100", "GenuineIntel")
CORTEX_A76 = CPU("Cortex-A76", "ARM", "arm64")
CELERON_N5105 = CPU("Intel(R) Celeron(R) N5105 @ 2.00GHz", "GenuineIntel")
RYZEN_7700X = CPU("AMD Ryzen 7 7700X 8-Core Processor", "AuthenticAMD")
CORE_I3_12100T = CPU("12th Gen Intel(R) Core(TM) i3-12100T", "GenuineIntel")
APPLE_M4 = CPU("Apple M4", "Apple", "arm64")
CELERON_J4125 = CPU("Intel(R) Celeron(R) J4125 CPU @ 2.00GHz", "GenuineIntel")


@dataclass(frozen=True)
class Host:
    """One agent host. Asset IDs are host names, the way real agents register."""

    id: str
    platform: str
    os: OS
    group: str
    ip: str
    cpu: CPU
    cores: int
    threads: int
    ram_gb: int
    disk_gb: int
    cpu_base: float
    mem_base: float
    disk_pct: float
    net_mbit: float  # typical receive rate
    tags: list = field(default_factory=list)
    online: bool = True
    added_days: int = 200
    uptime_days: float = 30.0


HOSTS = [
    Host("pve01", "linux", PVE, "grp-compute", "10.0.10.11", RYZEN_5950X, 16, 32, 128, 480,
         34, 71, 52, 15, ["proxmox", "hypervisor"], added_days=320, uptime_days=41),
    Host("pve02", "linux", PVE, "grp-compute", "10.0.10.12", CORE_I7_11700, 8, 16, 64, 480,
         27, 58, 44, 8, ["proxmox", "hypervisor"], added_days=320, uptime_days=41),
    Host("nas01", "linux", TRUENAS, "grp-storage", "10.0.10.20", CORE_I5_12400, 6, 12, 64, 120,
         22, 68, 91, 45, ["truenas", "zfs", "nfs"], added_days=310, uptime_days=63),
    Host("pbs01", "linux", PBS, "grp-storage", "10.0.10.21", CORE_I7_7700, 4, 8, 16, 4000,
         9, 42, 66, 5, ["backup", "pbs"], added_days=240, uptime_days=41),
    Host("vyos", "linux", VYOS, "grp-network", "10.0.10.1", INTEL_N100, 4, 4, 8, 32,
         8, 31, 18, 120, ["router", "firewall", "vpn"], added_days=365, uptime_days=5),
    Host("pihole", "linux", RASPBERRY_PI_OS, "grp-network", "10.0.10.2", CORTEX_A76, 4, 4, 4, 32,
         6, 28, 15, 0.8, ["dns", "dhcp"], added_days=365, uptime_days=88),
    Host("docker01", "linux", DEBIAN, "grp-services", "10.0.10.30", RYZEN_5950X, 8, 8, 32, 200,
         22, 62, 48, 8, ["docker"], added_days=260, uptime_days=3),
    Host("media01", "linux", UBUNTU, "grp-services", "10.0.10.31", CORE_I7_11700, 12, 12, 32, 500,
         21, 77, 55, 30, ["docker", "media"], added_days=190, uptime_days=22),
    Host("monitor01", "linux", DEBIAN, "grp-services", "10.0.10.32", CORE_I7_11700, 4, 4, 16, 200,
         18, 55, 62, 5, ["docker", "monitoring"], added_days=230, uptime_days=41),
    Host("k3s-cp1", "linux", UBUNTU, "grp-services", "10.0.10.40", RYZEN_5950X, 4, 4, 8, 60,
         30, 58, 40, 12, ["kubernetes", "k3s"], added_days=130, uptime_days=17),
    Host("k3s-w1", "linux", UBUNTU, "grp-services", "10.0.10.41", CORE_I7_11700, 8, 8, 16, 100,
         35, 65, 38, 10, ["kubernetes", "k3s"], added_days=130, uptime_days=17),
    Host("homeassistant", "linux", HAOS, "grp-services", "10.0.10.50", CELERON_N5105, 4, 4, 8, 64,
         14, 48, 35, 1.5, ["home-assistant", "zigbee"], added_days=400, uptime_days=12),
    Host("minecraft", "linux", DEBIAN, "grp-services", "10.0.10.60", RYZEN_5950X, 4, 4, 12, 80,
         8, 62, 35, 1, ["games"], added_days=50, uptime_days=9),
    Host("ci-runner", "linux", UBUNTU, "grp-desks", "10.0.10.70", RYZEN_5950X, 8, 8, 16, 160,
         15, 40, 52, 3, ["ci"], added_days=95, uptime_days=17),
    Host("workstation", "linux", FEDORA, "grp-desks", "10.0.10.71", RYZEN_7700X, 8, 16, 64, 1000,
         55, 72, 68, 5, ["desktop"], online=False, added_days=70, uptime_days=2),
    Host("htpc", "windows", WINDOWS, "grp-desks", "10.0.10.72", CORE_I3_12100T, 4, 8, 16, 512,
         5, 45, 60, 2, ["windows", "media"], added_days=95, uptime_days=6),
    Host("studio", "darwin", MACOS, "grp-desks", "10.0.10.73", APPLE_M4, 10, 10, 32, 1000,
         12, 52, 47, 4, ["macos", "desktop"], added_days=45, uptime_days=11),
    Host("offsite-pbs", "linux", DEBIAN, "grp-offsite", "10.20.0.10", CELERON_J4125, 4, 4, 8, 8000,
         5, 30, 42, 0.5, ["backup", "offsite", "wireguard"], added_days=160, uptime_days=75),
]

HOST_IDS = {host.id for host in HOSTS}
ONLINE_HOSTS = [host for host in HOSTS if host.online]

# How long ago the offline workstation was last seen.
OFFLINE_SINCE_HOURS = 3


def host_metadata(host: Host) -> dict:
    """The heartbeat facts as strings, the way an agent sends them."""
    disk_total = host.disk_gb * GIB
    disk_available = int(disk_total * (100 - host.disk_pct) / 100)
    return {
        "agent_version": "v2.0.0",
        "cpu_architecture": host.cpu.arch,
        "cpu_cores_physical": str(host.cores),
        "cpu_model": host.cpu.model,
        "cpu_threads_logical": str(host.threads),
        "cpu_used_percent": f"{host.cpu_base:.2f}",
        "cpu_vendor": host.cpu.vendor,
        "disk_root_available_bytes": str(disk_available),
        "disk_root_total_bytes": str(disk_total),
        "disk_used_percent": f"{host.disk_pct:.2f}",
        "hostname": host.id,
        "ip_address": host.ip,
        "kernel_release": host.os.kernel,
        "memory_total_bytes": str(host.ram_gb * GIB),
        "memory_used_percent": f"{host.mem_base:.2f}",
        "os": host.os.family,
        "os_id": host.os.os_id,
        "os_name": host.os.name,
        "os_pretty_name": host.os.pretty,
        "os_version": host.os.version,
        "platform": host.platform,
        "uptime_seconds": str(int(host.uptime_days * 86400)),
    }


def groups_sql() -> str:
    created = ago(days=320)
    rows = [
        [gid, name, slug, parent, icon, order, "UTC", "", {}, created, created]
        for gid, name, slug, parent, icon, order in GROUPS
    ]
    return section("GROUPS", insert(
        "groups",
        ["id", "name", "slug", "parent_group_id", "icon", "sort_order", "timezone", "location",
         "metadata", "created_at", "updated_at"],
        rows, "ON CONFLICT (id) DO NOTHING"))


def assets_sql() -> str:
    rows = []
    for host in HOSTS:
        seen = ago(seconds=20) if host.online else ago(hours=OFFLINE_SINCE_HOURS)
        rows.append([
            host.id, "host", host.id, "agent", "online" if host.online else "offline", host.platform,
            host.ip, "agent", host.group, host.tags, host_metadata(host),
            ago(days=host.added_days), seen, seen,
        ])
    return section("ASSETS", insert(
        "assets",
        ["id", "type", "name", "source", "status", "platform", "host", "transport_type", "group_id",
         "tags", "metadata", "created_at", "updated_at", "last_seen_at"],
        rows, "ON CONFLICT (id) DO NOTHING"))


def check_asset(asset_id: str, where: str) -> None:
    if asset_id not in HOST_IDS:
        raise ValueError(f"{where}: unknown asset {asset_id!r}")


def check_group(group_id: str, where: str) -> None:
    if group_id not in GROUP_IDS:
        raise ValueError(f"{where}: unknown group {group_id!r}")


def check_fleet() -> None:
    for gid, _name, _slug, parent, _icon, _order in GROUPS:
        if parent is not None:
            check_group(parent, f"group {gid}")
    for host in HOSTS:
        check_group(host.group, f"host {host.id}")
