"""Tests for LabTether binary sensor entities."""

import pytest
from unittest.mock import MagicMock

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.binary_sensor import (
    LabTetherAssetStatusSensor,
    LabTetherHubStatusSensor,
    async_setup_entry,
)
from labtether.coordinator import LabTetherData


def _make_coordinator(assets, metrics=None, alerts=0):
    """Create a mock coordinator with data."""
    coord = MagicMock()
    coord.entry_id = "entry-1"
    coord.data = LabTetherData(assets=assets, metrics=metrics or {}, firing_alerts_count=alerts)
    coord.last_update_success = True
    coord.api = MagicMock()
    coord.api._host = "http://192.168.1.10:8080"
    return coord


def test_asset_status_online():
    """Asset with status 'online' should be on."""
    coord = _make_coordinator([{"id": "a1", "name": "Node1", "type": "hypervisor-node", "source": "proxmox", "status": "online", "metadata": {}}])
    sensor = LabTetherAssetStatusSensor(coord, "a1")
    assert sensor.is_on is True


def test_asset_status_offline():
    """Asset with status 'offline' should be off."""
    coord = _make_coordinator([{"id": "a1", "name": "Node1", "type": "hypervisor-node", "source": "proxmox", "status": "offline", "metadata": {}}])
    sensor = LabTetherAssetStatusSensor(coord, "a1")
    assert sensor.is_on is False


def test_asset_status_stale_is_off():
    """Asset with status 'stale' should be off."""
    coord = _make_coordinator([{"id": "a1", "name": "Node1", "type": "vm", "source": "proxmox", "status": "stale", "metadata": {}}])
    sensor = LabTetherAssetStatusSensor(coord, "a1")
    assert sensor.is_on is False


def test_hub_status_reflects_coordinator():
    """Hub status should reflect coordinator update success."""
    coord = _make_coordinator([])
    sensor = LabTetherHubStatusSensor(coord)
    assert sensor.is_on is True

    coord.last_update_success = False
    assert sensor.is_on is False


@pytest.mark.asyncio
async def test_binary_sensor_setup_adds_new_assets_after_initial_load():
    """New assets discovered later should get status entities without a reload."""
    coord = _make_coordinator([
        {"id": "a1", "name": "Node1", "type": "hypervisor-node", "source": "proxmox", "status": "online", "metadata": {}}
    ])
    listeners = []
    coord.async_add_listener = MagicMock(side_effect=lambda cb: listeners.append(cb) or (lambda: None))

    hass = MagicMock()
    entry = MagicMock()
    entry.entry_id = "entry-1"
    entry.options = {}
    entry.data = {}
    entry.async_on_unload = MagicMock()
    hass.data = {"labtether": {"entry-1": {"coordinator": coord}}}

    added_batches = []

    def _add_entities(entities):
        added_batches.append(list(entities))

    await async_setup_entry(hass, entry, _add_entities)
    assert len(added_batches[0]) == 2  # hub + first asset
    assert added_batches[0][0]._attr_unique_id == "labtether_entry-1_hub_status"

    coord.data.assets.append(
        {"id": "a2", "name": "Node2", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}
    )
    listeners[0]()

    assert len(added_batches[1]) == 1
    assert added_batches[1][0]._asset_id == "a2"
