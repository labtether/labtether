"""Tests for the LabTether DataUpdateCoordinator."""

import pytest
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.coordinator import LabTetherData


def test_labtether_data_structure():
    """LabTetherData should hold assets, metrics, and alert count."""
    data = LabTetherData(
        assets=[{"id": "a1", "name": "A1", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}}],
        metrics={"a1": {"cpu_used_percent": 50.0}},
        firing_alerts_count=3,
    )
    assert len(data.assets) == 1
    assert data.metrics["a1"]["cpu_used_percent"] == 50.0
    assert data.firing_alerts_count == 3


def test_labtether_data_get_asset():
    """get_asset should look up by ID."""
    data = LabTetherData(
        assets=[
            {"id": "a1", "name": "A1", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}},
            {"id": "a2", "name": "A2", "type": "vm", "source": "proxmox", "status": "offline", "metadata": {}},
        ],
        metrics={},
        firing_alerts_count=0,
    )
    assert data.get_asset("a1")["name"] == "A1"
    assert data.get_asset("missing") is None


def test_labtether_data_get_metrics():
    """get_metrics should return metrics dict for asset or empty dict."""
    data = LabTetherData(
        assets=[],
        metrics={"a1": {"cpu_used_percent": 50.0}},
        firing_alerts_count=0,
    )
    assert data.get_metrics("a1")["cpu_used_percent"] == 50.0
    assert data.get_metrics("missing") == {}
