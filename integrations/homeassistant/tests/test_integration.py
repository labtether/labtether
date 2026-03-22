"""End-to-end integration test for LabTether HA integration."""

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.coordinator import LabTetherData
from labtether.const import DOMAIN, CONTROLLABLE_KINDS, TELEMETRY_KINDS, EXCLUDED_SOURCE


def test_domain_is_labtether():
    assert DOMAIN == "labtether"


def test_controllable_kinds_are_vms_and_containers():
    assert "vm" in CONTROLLABLE_KINDS
    assert "container" in CONTROLLABLE_KINDS
    assert "hypervisor-node" not in CONTROLLABLE_KINDS


def test_telemetry_kinds_include_compute():
    assert "hypervisor-node" in TELEMETRY_KINDS
    assert "vm" in TELEMETRY_KINDS
    assert "container" in TELEMETRY_KINDS


def test_excluded_source_is_homeassistant():
    assert EXCLUDED_SOURCE == "home-assistant"


def test_data_model_full_workflow():
    """Simulate a full data cycle."""
    assets = [
        {"id": "pve-node-1", "name": "DeltaServer", "type": "hypervisor-node", "source": "proxmox", "status": "online", "metadata": {}},
        {"id": "pve-vm-100", "name": "TestVM", "type": "vm", "source": "proxmox", "status": "running", "metadata": {"vmid": "100"}},
        {"id": "truenas-pool-1", "name": "MainPool", "type": "storage-pool", "source": "truenas", "status": "online", "metadata": {}},
    ]
    metrics = {
        "pve-node-1": {"cpu_used_percent": 45.0, "memory_used_percent": 72.0, "disk_used_percent": 30.0},
        "pve-vm-100": {"cpu_used_percent": 12.0, "memory_used_percent": 55.0, "disk_used_percent": 80.0},
    }
    data = LabTetherData(assets=assets, metrics=metrics, firing_alerts_count=2)

    # Verify asset lookup
    assert data.get_asset("pve-node-1")["name"] == "DeltaServer"
    assert data.get_asset("missing") is None

    # Verify metrics lookup
    assert data.get_metrics("pve-node-1")["cpu_used_percent"] == 45.0
    assert data.get_metrics("truenas-pool-1") == {}

    # Verify counts
    assert len(data.assets) == 3
    assert data.firing_alerts_count == 2

    # Verify which assets get switches (only VMs/containers)
    switchable = [a for a in data.assets if a["type"] in CONTROLLABLE_KINDS]
    assert len(switchable) == 1
    assert switchable[0]["id"] == "pve-vm-100"

    # Verify which assets get telemetry sensors
    with_telemetry = [a for a in data.assets if a["type"] in TELEMETRY_KINDS]
    assert len(with_telemetry) == 2  # node + VM, not storage-pool
