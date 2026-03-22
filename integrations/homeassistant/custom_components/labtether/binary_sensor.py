"""Binary sensor platform for LabTether."""

from __future__ import annotations

from homeassistant.components.binary_sensor import (
    BinarySensorDeviceClass,
    BinarySensorEntity,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .const import DOMAIN, CONF_IMPORT_BINARY_SENSORS, DEFAULT_IMPORT_BINARY_SENSORS, entry_pref
from .coordinator import LabTetherCoordinator
from .entity import LabTetherEntity, LabTetherHubEntity

PARALLEL_UPDATES = 0


async def async_setup_entry(
    hass: HomeAssistant,
    entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up LabTether binary sensors."""
    if not entry_pref(entry, CONF_IMPORT_BINARY_SENSORS, DEFAULT_IMPORT_BINARY_SENSORS):
        return

    coordinator: LabTetherCoordinator = hass.data[DOMAIN][entry.entry_id]["coordinator"]

    entities: list[BinarySensorEntity] = [
        LabTetherHubStatusSensor(coordinator),
    ]

    known_asset_ids: set[str] = set()

    def _current_asset_ids() -> set[str]:
        return {asset["id"] for asset in coordinator.data.assets if "id" in asset}

    for asset in coordinator.data.assets:
        entities.append(LabTetherAssetStatusSensor(coordinator, asset["id"]))
        known_asset_ids.add(asset["id"])

    async_add_entities(entities)

    @callback
    def _async_add_new_entities() -> None:
        nonlocal known_asset_ids
        current_asset_ids = _current_asset_ids()
        new_asset_ids = sorted(current_asset_ids - known_asset_ids)
        if not new_asset_ids:
            return
        async_add_entities(
            [LabTetherAssetStatusSensor(coordinator, asset_id) for asset_id in new_asset_ids]
        )
        known_asset_ids |= set(new_asset_ids)

    entry.async_on_unload(coordinator.async_add_listener(_async_add_new_entities))


class LabTetherHubStatusSensor(LabTetherHubEntity, BinarySensorEntity):
    """Binary sensor for hub reachability."""

    _attr_device_class = BinarySensorDeviceClass.CONNECTIVITY
    _attr_name = "Status"

    def __init__(self, coordinator: LabTetherCoordinator) -> None:
        super().__init__(coordinator)
        self._attr_unique_id = f"{DOMAIN}_{coordinator.entry_id}_hub_status"

    @property
    def is_on(self) -> bool:
        """Return True if the hub is reachable."""
        return self.coordinator.last_update_success


class LabTetherAssetStatusSensor(LabTetherEntity, BinarySensorEntity):
    """Binary sensor for asset online/offline status."""

    _attr_device_class = BinarySensorDeviceClass.CONNECTIVITY
    _attr_name = "Status"

    def __init__(self, coordinator: LabTetherCoordinator, asset_id: str) -> None:
        super().__init__(coordinator, asset_id)
        self._attr_unique_id = f"{DOMAIN}_{coordinator.entry_id}_{asset_id}_status"

    @property
    def is_on(self) -> bool:
        """Return True if the asset is online."""
        asset = self._asset
        if asset is None:
            return False
        return asset.get("status") == "online"
