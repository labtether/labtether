"""Runtime behavior tests for the LabTether integration bootstrap."""

from unittest.mock import AsyncMock, MagicMock

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

import pytest

import labtether as integration
from labtether.coordinator import LabTetherData
from labtether.const import asset_registry_key, hub_registry_key


def test_select_service_target_returns_matching_entry():
    """Service routing should choose the entry that actually owns the asset."""
    coordinator_a = MagicMock()
    coordinator_a.data = LabTetherData(
        assets=[{"id": "a1", "name": "A1", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}],
        metrics={},
        firing_alerts_count=0,
    )
    coordinator_b = MagicMock()
    coordinator_b.data = LabTetherData(
        assets=[{"id": "b1", "name": "B1", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}],
        metrics={},
        firing_alerts_count=0,
    )
    hass = MagicMock()
    hass.data = {
        "labtether": {
            "entry-a": {"coordinator": coordinator_a},
            "entry-b": {"coordinator": coordinator_b},
        }
    }

    selected = integration._select_service_target(hass, "b1")

    assert selected["coordinator"] is coordinator_b


def test_legacy_unique_id_migrations_namespace_old_ids():
    """Older global unique ids should migrate to per-entry namespaced ones."""
    coordinator = MagicMock()
    coordinator.entry_id = "entry-1"
    coordinator.data = LabTetherData(
        assets=[{"id": "vm-100", "name": "VM 100", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}],
        metrics={},
        firing_alerts_count=0,
    )

    migrations = integration._legacy_unique_id_migrations(coordinator)

    assert migrations[("binary_sensor", "labtether_hub_status")] == "labtether_entry-1_hub_status"
    assert migrations[("sensor", "labtether_vm-100_cpu_used_percent")] == "labtether_entry-1_vm-100_cpu_used_percent"
    assert migrations[("switch", "labtether_vm-100_power")] == "labtether_entry-1_vm-100_power"


def test_migrate_entity_unique_ids_updates_entity_registry():
    """Setup migration should rewrite older entity unique ids before platform setup."""
    hass = MagicMock()
    entry = MagicMock()
    entry.entry_id = "entry-1"

    coordinator = MagicMock()
    coordinator.entry_id = "entry-1"
    coordinator.data = LabTetherData(
        assets=[{"id": "vm-100", "name": "VM 100", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}],
        metrics={},
        firing_alerts_count=0,
    )

    entity_registry = MagicMock()
    old_entry = MagicMock()
    old_entry.platform = "binary_sensor"
    old_entry.unique_id = "labtether_vm-100_status"
    old_entry.entity_id = "binary_sensor.vm_100_status"
    integration.er.async_get = MagicMock(return_value=entity_registry)
    integration.er.async_entries_for_config_entry = MagicMock(return_value=[old_entry])

    integration._migrate_entity_unique_ids(hass, entry, coordinator)

    entity_registry.async_update_entity.assert_called_once_with(
        "binary_sensor.vm_100_status",
        new_unique_id="labtether_entry-1_vm-100_status",
    )


@pytest.mark.asyncio
async def test_remove_config_entry_device_blocks_hub_device_removal():
    """The synthetic hub device should not be removable."""
    hass = MagicMock()
    entry = MagicMock()
    entry.entry_id = "entry-1"
    device_entry = MagicMock()
    device_entry.identifiers = {("labtether", hub_registry_key("entry-1"))}

    assert await integration.async_remove_config_entry_device(hass, entry, device_entry) is False


@pytest.mark.asyncio
async def test_remove_config_entry_device_allows_stale_asset_removal():
    """Assets not present in current coordinator data should be removable."""
    hass = MagicMock()
    entry = MagicMock()
    entry.entry_id = "entry-1"
    coordinator = MagicMock()
    coordinator.data = LabTetherData(
        assets=[{"id": "a1", "name": "A1", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}],
        metrics={},
        firing_alerts_count=0,
    )
    hass.data = {"labtether": {"entry-1": {"coordinator": coordinator}}}

    stale_device = MagicMock()
    stale_device.identifiers = {("labtether", asset_registry_key("entry-1", "missing"))}

    assert await integration.async_remove_config_entry_device(hass, entry, stale_device) is True


@pytest.mark.asyncio
async def test_setup_entry_ensures_hub_device_exists():
    """Setup should create the hub device even when hub entities are disabled."""
    hass = MagicMock()
    hass.data = {}
    hass.services.has_service.return_value = False
    hass.config_entries.async_forward_entry_setups = AsyncMock()
    device_registry = MagicMock()

    entry = MagicMock()
    entry.entry_id = "entry-1"
    entry.data = {"host": "https://lab.local:8443", "api_key": "token"}
    entry.options = {}
    entry.async_on_unload = MagicMock()
    entry.add_update_listener = MagicMock(return_value=lambda: None)

    coordinator = MagicMock()
    coordinator.async_config_entry_first_refresh = AsyncMock()

    integration.dr.async_get = MagicMock(return_value=device_registry)
    integration._build_client = MagicMock(return_value=MagicMock(host="https://lab.local:8443"))
    integration.LabTetherCoordinator = MagicMock(return_value=coordinator)

    result = await integration.async_setup_entry(hass, entry)

    assert result is True
    device_registry.async_get_or_create.assert_called_once()
