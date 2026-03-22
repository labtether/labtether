"""Constants for the LabTether integration."""

DOMAIN = "labtether"

CONF_HOST = "host"
CONF_API_KEY = "api_key"
CONF_NAME = "name"
CONF_IGNORE_CERT_ERRORS = "ignore_cert_errors"
CONF_IMPORT_BINARY_SENSORS = "import_status_entities"
CONF_IMPORT_SENSORS = "import_telemetry_sensors"
CONF_IMPORT_SWITCHES = "import_power_switches"
CONF_ENABLE_RUN_ACTION_SERVICE = "enable_run_action_service"
CONF_SCAN_INTERVAL = "scan_interval_seconds"

DEFAULT_SCAN_INTERVAL = 30  # seconds
DEFAULT_IMPORT_BINARY_SENSORS = True
DEFAULT_IMPORT_SENSORS = True
DEFAULT_IMPORT_SWITCHES = True
DEFAULT_ENABLE_RUN_ACTION_SERVICE = True

# LabTether API endpoints
API_ASSETS = "/assets"
API_METRICS_OVERVIEW = "/metrics/overview"
API_ALERTS_INSTANCES = "/alerts/instances"
API_ACTIONS_EXECUTE = "/actions/execute"

# HA platforms to set up
PLATFORMS = ["sensor", "binary_sensor", "switch"]

# Asset kinds that support power control (start/stop)
CONTROLLABLE_KINDS = {"vm", "container"}
POWER_ACTION_SOURCES = {"proxmox", "truenas", "docker"}

# Asset source to exclude (prevents circular entity mirroring)
EXCLUDED_SOURCE = "home-assistant"

# Asset kinds that typically have telemetry
TELEMETRY_KINDS = {"hypervisor-node", "vm", "container", "container-host"}


def entry_pref(entry, key: str, default):
    """Return a config entry preference, favoring options over stored data."""
    return entry.options.get(key, entry.data.get(key, default))


def hub_registry_key(entry_id: str) -> str:
    """Return the hub device registry key for a config entry."""
    return f"{entry_id}:hub"


def asset_registry_key(entry_id: str, asset_id: str) -> str:
    """Return the asset device registry key for a config entry."""
    return f"{entry_id}:asset:{asset_id}"
