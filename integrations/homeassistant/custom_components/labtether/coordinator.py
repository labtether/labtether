"""DataUpdateCoordinator for LabTether."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import timedelta
from typing import TYPE_CHECKING, Any

from .const import DOMAIN, DEFAULT_SCAN_INTERVAL

if TYPE_CHECKING:
    from homeassistant.core import HomeAssistant

_LOGGER = logging.getLogger(__name__)


@dataclass
class LabTetherData:
    """Data class holding all LabTether state."""

    assets: list[dict[str, Any]] = field(default_factory=list)
    metrics: dict[str, dict[str, Any]] = field(default_factory=dict)
    firing_alerts_count: int = 0

    def get_asset(self, asset_id: str) -> dict[str, Any] | None:
        """Get an asset by ID."""
        for asset in self.assets:
            if asset.get("id") == asset_id:
                return asset
        return None

    def get_metrics(self, asset_id: str) -> dict[str, Any]:
        """Get metrics for an asset, or empty dict if none."""
        return self.metrics.get(asset_id, {})


def _build_coordinator_class() -> type:
    """Build LabTetherCoordinator class when homeassistant is available."""
    from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed
    from homeassistant.exceptions import ConfigEntryAuthFailed
    from .api import LabTetherApiClient, LabTetherApiError

    class LabTetherCoordinator(DataUpdateCoordinator[LabTetherData]):
        """Coordinator to poll LabTether API."""

        def __init__(
            self,
            hass: HomeAssistant,
            api: LabTetherApiClient,
            entry_id: str,
            scan_interval_seconds: int = DEFAULT_SCAN_INTERVAL,
        ) -> None:
            super().__init__(
                hass,
                _LOGGER,
                name=DOMAIN,
                update_interval=timedelta(seconds=scan_interval_seconds),
            )
            self.api = api
            self.entry_id = entry_id

        async def _async_update_data(self) -> LabTetherData:
            """Fetch data from LabTether API."""
            try:
                assets = await self.api.async_get_assets()
                metrics = await self.api.async_get_metrics_overview()
                alerts_count = await self.api.async_get_firing_alerts_count()
            except LabTetherApiError as err:
                if "Authentication failed" in str(err):
                    raise ConfigEntryAuthFailed from err
                raise UpdateFailed(f"Error communicating with LabTether: {err}") from err

            return LabTetherData(
                assets=assets,
                metrics=metrics,
                firing_alerts_count=alerts_count,
            )

    return LabTetherCoordinator


try:
    LabTetherCoordinator = _build_coordinator_class()
except ImportError:
    # homeassistant not installed — coordinator unavailable outside HA runtime.
    # LabTetherData remains importable for standalone testing.
    LabTetherCoordinator = None  # type: ignore[assignment,misc]
