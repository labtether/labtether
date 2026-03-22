"""Tests for LabTether switch entities."""

import pytest
from unittest.mock import MagicMock, AsyncMock

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.switch import LabTetherPowerSwitch, async_setup_entry
from labtether.coordinator import LabTetherData


def _make_coordinator(assets, metrics=None):
    coord = MagicMock()
    coord.entry_id = "entry-1"
    coord.data = LabTetherData(assets=assets, metrics=metrics or {}, firing_alerts_count=0)
    coord.last_update_success = True
    coord.api = MagicMock()
    coord.api.host = "http://192.168.1.10:8080"
    coord.api.async_run_action = AsyncMock()
    coord.async_request_refresh = AsyncMock()
    return coord


def test_power_switch_is_on_when_online():
    """Switch should be on when asset status is online/running."""
    coord = _make_coordinator([{"id": "vm-1", "name": "TestVM", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}])
    switch = LabTetherPowerSwitch(coord, "vm-1")
    assert switch.is_on is True


def test_power_switch_is_off_when_offline():
    """Switch should be off when asset status is offline/stopped."""
    coord = _make_coordinator([{"id": "vm-1", "name": "TestVM", "type": "vm", "source": "proxmox", "status": "offline", "metadata": {}}])
    switch = LabTetherPowerSwitch(coord, "vm-1")
    assert switch.is_on is False


@pytest.mark.asyncio
async def test_turn_on_calls_start_action():
    """Turning on should call vm.start action."""
    coord = _make_coordinator([{"id": "vm-1", "name": "TestVM", "type": "vm", "source": "proxmox", "status": "offline", "metadata": {}}])
    switch = LabTetherPowerSwitch(coord, "vm-1")
    await switch.async_turn_on()
    coord.api.async_run_action.assert_called_once_with(
        asset_id="vm-1",
        action_type="connector_action",
        connector_id="proxmox",
        action_id="vm.start",
    )


@pytest.mark.asyncio
async def test_turn_off_calls_stop_action():
    """Turning off should call vm.stop action."""
    coord = _make_coordinator([{"id": "vm-1", "name": "TestVM", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}])
    switch = LabTetherPowerSwitch(coord, "vm-1")
    await switch.async_turn_off()
    coord.api.async_run_action.assert_called_once_with(
        asset_id="vm-1",
        action_type="connector_action",
        connector_id="proxmox",
        action_id="vm.stop",
    )


@pytest.mark.asyncio
async def test_switch_setup_skips_unsupported_sources_and_adds_new_supported_assets():
    """Only supported sources should get power switches, including newly discovered ones."""
    coord = _make_coordinator([
        {"id": "vm-1", "name": "VM1", "type": "vm", "source": "unknown", "status": "offline", "metadata": {}}
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
    assert added_batches[0] == []

    coord.data.assets.append(
        {"id": "vm-2", "name": "VM2", "type": "vm", "source": "proxmox", "status": "offline", "metadata": {}}
    )
    listeners[0]()

    assert len(added_batches[1]) == 1
    assert added_batches[1][0]._asset_id == "vm-2"
    assert added_batches[1][0]._attr_unique_id == "labtether_entry-1_vm-2_power"
