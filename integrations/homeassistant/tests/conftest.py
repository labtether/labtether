"""Shared fixtures for LabTether HA integration tests.

Mocks the homeassistant package so tests can run without HA installed.
"""

import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock


def _create_mock_module(name: str) -> ModuleType:
    """Create a mock module that returns MagicMock for any attribute access."""
    mod = ModuleType(name)
    mod.__dict__["__path__"] = []
    mod.__dict__["__file__"] = f"<mock {name}>"
    return mod


# Mock homeassistant and all its submodules before any imports
_HA_MODULES = [
    "homeassistant",
    "homeassistant.config_entries",
    "homeassistant.core",
    "homeassistant.const",
    "homeassistant.exceptions",
    "homeassistant.helpers",
    "homeassistant.helpers.aiohttp_client",
    "homeassistant.helpers.config_validation",
    "homeassistant.helpers.device_registry",
    "homeassistant.helpers.entity_registry",
    "homeassistant.helpers.entity_platform",
    "homeassistant.helpers.update_coordinator",
    "homeassistant.components",
    "homeassistant.components.binary_sensor",
    "homeassistant.components.sensor",
    "homeassistant.components.switch",
]

for mod_name in _HA_MODULES:
    if mod_name not in sys.modules:
        sys.modules[mod_name] = MagicMock()

# Wire parent-child module relationships so `from homeassistant import X` works
# (MagicMock.__getattr__ would otherwise return a new mock instead of sys.modules child)
_ha = sys.modules["homeassistant"]
_ha.config_entries = sys.modules["homeassistant.config_entries"]
_ha.core = sys.modules["homeassistant.core"]
_ha.const = sys.modules["homeassistant.const"]
_ha.exceptions = sys.modules["homeassistant.exceptions"]
_ha.helpers = sys.modules["homeassistant.helpers"]
_ha.components = sys.modules["homeassistant.components"]
_ha_helpers = sys.modules["homeassistant.helpers"]
_ha_helpers.aiohttp_client = sys.modules["homeassistant.helpers.aiohttp_client"]
_ha_helpers.config_validation = sys.modules["homeassistant.helpers.config_validation"]
_ha_helpers.device_registry = sys.modules["homeassistant.helpers.device_registry"]
_ha_helpers.entity_registry = sys.modules["homeassistant.helpers.entity_registry"]
_ha_helpers.entity_platform = sys.modules["homeassistant.helpers.entity_platform"]
_ha_helpers.update_coordinator = sys.modules["homeassistant.helpers.update_coordinator"]
_ha_components = sys.modules["homeassistant.components"]
_ha_components.binary_sensor = sys.modules["homeassistant.components.binary_sensor"]
_ha_components.sensor = sys.modules["homeassistant.components.sensor"]
_ha_components.switch = sys.modules["homeassistant.components.switch"]

# Set specific values that our code actually uses
ha_const = sys.modules["homeassistant.const"]
ha_const.PERCENTAGE = "%"

# Make ConfigFlow usable as a base class
ha_config_entries = sys.modules["homeassistant.config_entries"]


class _ConfigFlow:
    def __init__(self):
        self.hass = MagicMock()
        self.context = {}

    def __init_subclass__(cls, **kw):
        super().__init_subclass__()

    async def async_set_unique_id(self, *a, **kw):
        return None

    def _abort_if_unique_id_configured(self, *a, **kw):
        return None

    def async_create_entry(self, **kw):
        return {"type": "create_entry", **kw}

    def async_show_form(self, **kw):
        return {"type": "form", **kw}

    def async_abort(self, **kw):
        return {"type": "abort", **kw}

    def _async_current_entries(self):
        config_entries = getattr(self.hass, "config_entries", None)
        if config_entries is None or not hasattr(config_entries, "async_entries"):
            return []
        return config_entries.async_entries()

    def _get_reauth_entry(self):
        entry_id = self.context.get("entry_id")
        if entry_id is None:
            raise KeyError("entry_id")
        return self.hass.config_entries.async_get_entry(entry_id)

    def _get_reconfigure_entry(self):
        entry_id = self.context.get("entry_id")
        if entry_id is None:
            raise KeyError("entry_id")
        return self.hass.config_entries.async_get_entry(entry_id)

    def async_update_reload_and_abort(self, entry, **kw):
        data = kw.pop("data", None)
        data_updates = kw.pop("data_updates", None)
        options = kw.pop("options", None)
        title = kw.pop("title", None)
        if data_updates is not None:
            data = {**dict(entry.data), **dict(data_updates)}
        update_kwargs = {}
        if data is not None:
            update_kwargs["data"] = data
        if options is not None:
            update_kwargs["options"] = options
        if title is not None:
            update_kwargs["title"] = title
        if update_kwargs:
            self.hass.config_entries.async_update_entry(entry, **update_kwargs)
        if hasattr(self.hass.config_entries, "async_schedule_reload"):
            self.hass.config_entries.async_schedule_reload(entry.entry_id)
        return {"type": "abort", "reason": kw.get("reason", "updated")}


ha_config_entries.ConfigFlow = _ConfigFlow
ha_config_entries.OptionsFlow = _ConfigFlow
ha_config_entries.ConfigEntry = MagicMock

# Make CoordinatorEntity usable as a base class (must support Generic[] subscript)
ha_coordinator = sys.modules["homeassistant.helpers.update_coordinator"]


class _CoordinatorEntity:
    def __init__(self, coordinator, **kw):
        self.coordinator = coordinator

    def __init_subclass__(cls, **kw):
        super().__init_subclass__(**kw)

    def __class_getitem__(cls, item):
        return cls


class _DataUpdateCoordinator:
    def __init__(self, *a, **kw):
        pass

    def __class_getitem__(cls, item):
        return cls


ha_coordinator.CoordinatorEntity = _CoordinatorEntity
ha_coordinator.DataUpdateCoordinator = _DataUpdateCoordinator
ha_coordinator.UpdateFailed = Exception

# Make entity base classes usable
ha_binary_sensor = sys.modules["homeassistant.components.binary_sensor"]
ha_binary_sensor.BinarySensorEntity = type("BinarySensorEntity", (), {})
ha_binary_sensor.BinarySensorDeviceClass = MagicMock()

ha_sensor = sys.modules["homeassistant.components.sensor"]
ha_sensor.SensorEntity = type("SensorEntity", (), {})
ha_sensor.SensorDeviceClass = MagicMock()
ha_sensor.SensorStateClass = MagicMock()

ha_switch = sys.modules["homeassistant.components.switch"]
ha_switch.SwitchEntity = type("SwitchEntity", (), {})
ha_switch.SwitchDeviceClass = MagicMock()

ha_device_registry = sys.modules["homeassistant.helpers.device_registry"]
ha_device_registry.DeviceInfo = dict  # DeviceInfo is basically a TypedDict
ha_device_registry.DeviceEntry = type("DeviceEntry", (), {})
ha_device_registry.async_get = lambda hass: MagicMock()

ha_entity_registry = sys.modules["homeassistant.helpers.entity_registry"]
ha_entity_registry.async_get = lambda hass: MagicMock()
ha_entity_registry.async_entries_for_config_entry = lambda registry, entry_id: []

ha_exceptions = sys.modules["homeassistant.exceptions"]
ha_exceptions.ConfigEntryAuthFailed = type("ConfigEntryAuthFailed", (Exception,), {})

ha_cv = sys.modules["homeassistant.helpers.config_validation"]
ha_cv.string = str

ha_aiohttp_client = sys.modules["homeassistant.helpers.aiohttp_client"]
ha_aiohttp_client.async_get_clientsession = lambda hass: MagicMock()

ha_core = sys.modules["homeassistant.core"]
ha_core.callback = lambda func: func

# Add custom_components to the path for imports
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))
