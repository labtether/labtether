"""Tests for LabTether entity identity and device metadata."""

from unittest.mock import MagicMock

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.binary_sensor import LabTetherAssetStatusSensor, LabTetherHubStatusSensor
from labtether.coordinator import LabTetherData
from labtether.sensor import LabTetherCpuSensor


def _make_coordinator(entry_id: str, assets, metrics=None):
    coord = MagicMock()
    coord.entry_id = entry_id
    coord.data = LabTetherData(assets=assets, metrics=metrics or {}, firing_alerts_count=0)
    coord.last_update_success = True
    coord.api = MagicMock()
    coord.api.host = "https://lab.local:8443"
    return coord


def test_multi_entry_unique_ids_are_namespaced():
    """Two hubs can expose the same asset id without entity unique-id collisions."""
    assets = [{"id": "vm-100", "name": "VM 100", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}]
    coord_a = _make_coordinator("entry-a", assets, metrics={"vm-100": {"cpu_used_percent": 10.0}})
    coord_b = _make_coordinator("entry-b", assets, metrics={"vm-100": {"cpu_used_percent": 20.0}})

    status_a = LabTetherAssetStatusSensor(coord_a, "vm-100")
    status_b = LabTetherAssetStatusSensor(coord_b, "vm-100")
    cpu_a = LabTetherCpuSensor(coord_a, "vm-100")
    cpu_b = LabTetherCpuSensor(coord_b, "vm-100")
    hub_a = LabTetherHubStatusSensor(coord_a)
    hub_b = LabTetherHubStatusSensor(coord_b)

    assert status_a._attr_unique_id != status_b._attr_unique_id
    assert cpu_a._attr_unique_id != cpu_b._attr_unique_id
    assert hub_a._attr_unique_id != hub_b._attr_unique_id


def test_multi_entry_device_identifiers_are_namespaced():
    """Two hubs can expose the same asset id without device-registry collisions."""
    assets = [{"id": "vm-100", "name": "VM 100", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}]
    coord_a = _make_coordinator("entry-a", assets)
    coord_b = _make_coordinator("entry-b", assets)

    device_info_a = LabTetherAssetStatusSensor(coord_a, "vm-100").device_info
    device_info_b = LabTetherAssetStatusSensor(coord_b, "vm-100").device_info
    hub_info_a = LabTetherHubStatusSensor(coord_a).device_info
    hub_info_b = LabTetherHubStatusSensor(coord_b).device_info

    assert device_info_a["identifiers"] != device_info_b["identifiers"]
    assert device_info_a["via_device"] != device_info_b["via_device"]
    assert hub_info_a["identifiers"] != hub_info_b["identifiers"]
