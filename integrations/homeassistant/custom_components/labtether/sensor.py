"""Sensor platform for LabTether."""

from __future__ import annotations

from homeassistant.components.sensor import (
    SensorEntity,
    SensorStateClass,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import PERCENTAGE
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .const import (
    DOMAIN,
    TELEMETRY_KINDS,
    CONF_IMPORT_SENSORS,
    DEFAULT_IMPORT_SENSORS,
    entry_pref,
)
from .coordinator import LabTetherCoordinator
from .entity import LabTetherEntity, LabTetherHubEntity

PARALLEL_UPDATES = 0


async def async_setup_entry(
    hass: HomeAssistant,
    entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up LabTether sensors."""
    if not entry_pref(entry, CONF_IMPORT_SENSORS, DEFAULT_IMPORT_SENSORS):
        return

    coordinator: LabTetherCoordinator = hass.data[DOMAIN][entry.entry_id]["coordinator"]

    entities: list[SensorEntity] = [
        LabTetherTotalAssetsSensor(coordinator),
        LabTetherActiveAlertsSensor(coordinator),
    ]

    known_asset_ids: set[str] = set()

    def _telemetry_asset_ids() -> set[str]:
        return {
            asset["id"]
            for asset in coordinator.data.assets
            if asset.get("type", "") in TELEMETRY_KINDS and "id" in asset
        }

    for asset in coordinator.data.assets:
        asset_id = asset["id"]
        asset_type = asset.get("type", "")

        if asset_type in TELEMETRY_KINDS:
            entities.append(LabTetherCpuSensor(coordinator, asset_id))
            entities.append(LabTetherMemorySensor(coordinator, asset_id))
            entities.append(LabTetherDiskSensor(coordinator, asset_id))
            known_asset_ids.add(asset_id)

    async_add_entities(entities)

    @callback
    def _async_add_new_entities() -> None:
        nonlocal known_asset_ids
        current_asset_ids = _telemetry_asset_ids()
        new_asset_ids = sorted(current_asset_ids - known_asset_ids)
        if not new_asset_ids:
            return

        new_entities: list[SensorEntity] = []
        for asset_id in new_asset_ids:
            new_entities.extend(
                [
                    LabTetherCpuSensor(coordinator, asset_id),
                    LabTetherMemorySensor(coordinator, asset_id),
                    LabTetherDiskSensor(coordinator, asset_id),
                ]
            )
        async_add_entities(new_entities)
        known_asset_ids |= set(new_asset_ids)

    entry.async_on_unload(coordinator.async_add_listener(_async_add_new_entities))


class LabTetherTotalAssetsSensor(LabTetherHubEntity, SensorEntity):
    """Sensor showing total number of assets."""

    _attr_name = "Total Assets"
    _attr_icon = "mdi:server-network"
    _attr_state_class = SensorStateClass.MEASUREMENT

    def __init__(self, coordinator: LabTetherCoordinator) -> None:
        super().__init__(coordinator)
        self._attr_unique_id = f"{DOMAIN}_{coordinator.entry_id}_hub_total_assets"

    @property
    def native_value(self) -> int:
        return len(self.coordinator.data.assets)


class LabTetherActiveAlertsSensor(LabTetherHubEntity, SensorEntity):
    """Sensor showing number of firing alerts."""

    _attr_name = "Active Alerts"
    _attr_icon = "mdi:alert-circle"
    _attr_state_class = SensorStateClass.MEASUREMENT

    def __init__(self, coordinator: LabTetherCoordinator) -> None:
        super().__init__(coordinator)
        self._attr_unique_id = f"{DOMAIN}_{coordinator.entry_id}_hub_active_alerts"

    @property
    def native_value(self) -> int:
        return self.coordinator.data.firing_alerts_count


class _LabTetherMetricSensor(LabTetherEntity, SensorEntity):
    """Base class for per-asset metric sensors."""

    # Utilization percentages do not map to a dedicated SensorDeviceClass.
    _attr_state_class = SensorStateClass.MEASUREMENT
    _attr_native_unit_of_measurement = PERCENTAGE
    _metric_key: str = ""

    def __init__(self, coordinator: LabTetherCoordinator, asset_id: str) -> None:
        super().__init__(coordinator, asset_id)
        self._attr_unique_id = f"{DOMAIN}_{coordinator.entry_id}_{asset_id}_{self._metric_key}"

    @property
    def native_value(self) -> float | None:
        return self._metrics.get(self._metric_key)


class LabTetherCpuSensor(_LabTetherMetricSensor):
    """CPU usage sensor."""

    _attr_name = "CPU Usage"
    _attr_icon = "mdi:cpu-64-bit"
    _metric_key = "cpu_used_percent"


class LabTetherMemorySensor(_LabTetherMetricSensor):
    """Memory usage sensor."""

    _attr_name = "Memory Usage"
    _attr_icon = "mdi:memory"
    _metric_key = "memory_used_percent"


class LabTetherDiskSensor(_LabTetherMetricSensor):
    """Disk usage sensor."""

    _attr_name = "Disk Usage"
    _attr_icon = "mdi:harddisk"
    _metric_key = "disk_used_percent"
