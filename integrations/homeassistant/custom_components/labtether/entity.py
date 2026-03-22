"""Base entity for LabTether integration."""

from __future__ import annotations

from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from .const import DOMAIN, asset_registry_key, hub_registry_key
from .coordinator import LabTetherCoordinator


class LabTetherEntity(CoordinatorEntity[LabTetherCoordinator]):
    """Base class for LabTether entities."""

    _attr_has_entity_name = True

    def __init__(self, coordinator: LabTetherCoordinator, asset_id: str) -> None:
        super().__init__(coordinator)
        self._asset_id = asset_id

    @property
    def _asset(self) -> dict | None:
        """Get the current asset data."""
        return self.coordinator.data.get_asset(self._asset_id)

    @property
    def _metrics(self) -> dict:
        """Get the current metrics for this asset."""
        return self.coordinator.data.get_metrics(self._asset_id)

    @property
    def available(self) -> bool:
        """Return True if the asset exists in the latest data."""
        return self.coordinator.last_update_success and self._asset is not None

    @property
    def device_info(self) -> DeviceInfo:
        """Return device info for this asset."""
        asset = self._asset
        entry_id = self.coordinator.entry_id
        if asset is None:
            return DeviceInfo(identifiers={(DOMAIN, asset_registry_key(entry_id, self._asset_id))})

        info = DeviceInfo(
            identifiers={(DOMAIN, asset_registry_key(entry_id, self._asset_id))},
            name=str(asset.get("name", self._asset_id)),
            manufacturer="LabTether",
            model=str(asset.get("type", "unknown")),
            sw_version=(
                str(asset.get("metadata", {}).get("version"))
                if asset.get("metadata", {}).get("version") is not None
                else None
            ),
        )
        # Link non-hub assets to the hub device
        if asset.get("type") != "hub":
            info["via_device"] = (DOMAIN, hub_registry_key(entry_id))
        return info


class LabTetherHubEntity(CoordinatorEntity[LabTetherCoordinator]):
    """Base class for hub-level entities."""

    _attr_has_entity_name = True

    def __init__(self, coordinator: LabTetherCoordinator) -> None:
        super().__init__(coordinator)

    @property
    def device_info(self) -> DeviceInfo:
        """Return device info for the hub."""
        return DeviceInfo(
            identifiers={(DOMAIN, hub_registry_key(self.coordinator.entry_id))},
            name="LabTether Hub",
            manufacturer="LabTether",
            model="Hub",
            configuration_url=self.coordinator.api.host,
        )
