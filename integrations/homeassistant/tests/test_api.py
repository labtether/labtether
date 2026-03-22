"""Tests for the LabTether API client."""

import pytest
from aiohttp import ClientSession
from unittest.mock import AsyncMock, MagicMock

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.api import LabTetherApiClient, LabTetherApiError


@pytest.fixture
def api_client():
    """Create an API client for testing."""
    return LabTetherApiClient(
        host="http://192.168.1.10:8080",
        api_key="test-token-123",
        session=AsyncMock(spec=ClientSession),
    )


def _mock_response(json_data, status=200):
    """Create a mock aiohttp response."""
    resp = AsyncMock()
    resp.status = status
    resp.content_type = "application/json"
    resp.json = AsyncMock(return_value=json_data)
    resp.text = AsyncMock(return_value=str(json_data))
    resp.__aenter__ = AsyncMock(return_value=resp)
    resp.__aexit__ = AsyncMock(return_value=False)
    return resp


@pytest.mark.asyncio
async def test_get_assets_returns_filtered_list(api_client):
    """Assets with source 'home-assistant' or no id should be excluded."""
    mock_data = {
        "assets": [
            {"id": "pve-node-1", "name": "Node1", "type": "hypervisor-node", "source": "proxmox", "status": "online", "metadata": {}},
            {"id": "ha-light-1", "name": "Light", "type": "ha-entity", "source": "home-assistant", "status": "online", "metadata": {}},
            {"name": "Broken", "type": "vm", "source": "proxmox", "status": "online", "metadata": {}},
        ]
    }
    api_client._session.request = MagicMock(return_value=_mock_response(mock_data))

    assets = await api_client.async_get_assets()
    assert len(assets) == 1
    assert assets[0]["id"] == "pve-node-1"


@pytest.mark.asyncio
async def test_get_metrics_overview(api_client):
    """Metrics overview should return asset metrics."""
    mock_data = {
        "assets": [
            {
                "asset_id": "pve-node-1",
                "metrics": {"cpu_used_percent": 45.5, "memory_used_percent": 72.0, "disk_used_percent": 30.0},
            }
        ]
    }
    api_client._session.request = MagicMock(return_value=_mock_response(mock_data))

    metrics = await api_client.async_get_metrics_overview()
    assert "pve-node-1" in metrics
    assert metrics["pve-node-1"]["cpu_used_percent"] == 45.5


@pytest.mark.asyncio
async def test_get_firing_alerts_count(api_client):
    """Should return the count of firing alerts."""
    mock_data = {
        "instances": [
            {"id": "alert-1", "status": "firing"},
            {"id": "alert-2", "status": "firing"},
        ]
    }
    api_client._session.request = MagicMock(return_value=_mock_response(mock_data))

    count = await api_client.async_get_firing_alerts_count()
    assert count == 2


@pytest.mark.asyncio
async def test_validate_connection_success(api_client):
    """Validate connection should return True on 200."""
    mock_data = {"assets": []}
    api_client._session.get = MagicMock(return_value=_mock_response(mock_data))

    result = await api_client.async_validate_connection()
    assert result is True


@pytest.mark.asyncio
async def test_validate_connection_failure(api_client):
    """Validate connection should return False on non-200."""
    api_client._session.get = MagicMock(return_value=_mock_response({}, status=401))

    result = await api_client.async_validate_connection()
    assert result is False


@pytest.mark.asyncio
async def test_run_action(api_client):
    """Run action should POST to actions/execute."""
    mock_resp = _mock_response({"job_id": "job-1", "status": "queued"}, status=202)
    api_client._session.request = MagicMock(return_value=mock_resp)

    result = await api_client.async_run_action("pve-vm-100", "connector_action", connector_id="proxmox", action_id="vm.start")
    assert result["job_id"] == "job-1"


@pytest.mark.asyncio
async def test_api_error_on_server_error(api_client):
    """Should raise LabTetherApiError on 500."""
    api_client._session.request = MagicMock(return_value=_mock_response({"error": "internal"}, status=500))

    with pytest.raises(LabTetherApiError):
        await api_client.async_get_assets()


@pytest.mark.asyncio
async def test_ignore_cert_errors_uses_ssl_false():
    """Self-signed hubs should be allowed when ignore_cert_errors is enabled."""
    session = AsyncMock(spec=ClientSession)
    session.request = MagicMock(return_value=_mock_response({"assets": []}))
    client = LabTetherApiClient(
        host="https://lab.local:8443",
        api_key="test-token-123",
        session=session,
        ignore_cert_errors=True,
    )

    await client.async_get_assets()

    assert session.request.call_args.kwargs["ssl"] is False


@pytest.mark.asyncio
async def test_setup_preview_summarizes_assets_metrics_and_alerts():
    """Setup preview should summarize discovered topology."""
    session = AsyncMock(spec=ClientSession)
    client = LabTetherApiClient(
        host="https://lab.local:8443",
        api_key="test-token-123",
        session=session,
    )
    client.async_get_assets = AsyncMock(return_value=[
        {"id": "node-1", "type": "hypervisor-node", "source": "proxmox"},
        {"id": "vm-1", "type": "vm", "source": "proxmox"},
        {"id": "pool-1", "type": "storage-pool", "source": "truenas"},
    ])
    client.async_get_metrics_overview = AsyncMock(return_value={
        "node-1": {"cpu_used_percent": 25.0},
        "vm-1": {"cpu_used_percent": 10.0},
    })
    client.async_get_firing_alerts_count = AsyncMock(return_value=3)

    preview = await client.async_get_setup_preview()

    assert preview["host_label"] == "lab.local:8443"
    assert preview["asset_count"] == 3
    assert preview["telemetry_asset_count"] == 2
    assert preview["metric_asset_count"] == 2
    assert preview["switchable_asset_count"] == 1
    assert preview["alerts_count"] == 3
