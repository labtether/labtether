"""Tests for the LabTether config flow logic."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent / "custom_components"))

from labtether.const import (
    CONF_API_KEY,
    CONF_ENABLE_RUN_ACTION_SERVICE,
    CONF_HOST,
    CONF_IGNORE_CERT_ERRORS,
    CONF_IMPORT_BINARY_SENSORS,
    CONF_IMPORT_SENSORS,
    CONF_IMPORT_SWITCHES,
    CONF_NAME,
    CONF_SCAN_INTERVAL,
    DOMAIN,
)


def test_config_flow_module_imports():
    """Config flow module should import cleanly."""
    from labtether.config_flow import LabTetherConfigFlow, USER_DATA_SCHEMA, IMPORT_OPTIONS_SCHEMA

    assert LabTetherConfigFlow is not None
    assert USER_DATA_SCHEMA is not None
    assert IMPORT_OPTIONS_SCHEMA is not None


def test_config_flow_data_schema_requires_host_and_key():
    """Connection schema should expose host and api_key fields."""
    from labtether.config_flow import USER_DATA_SCHEMA

    result = USER_DATA_SCHEMA({"host": "http://localhost:8080", "api_key": "test"})
    assert result["host"] == "http://localhost:8080"
    assert result["api_key"] == "test"
    assert "name" in result
    assert "ignore_cert_errors" in result


def test_config_flow_domain_is_set():
    """Config flow should be registered for the labtether domain."""
    assert DOMAIN == "labtether"


@pytest.mark.asyncio
async def test_config_flow_rejects_invalid_url():
    """User step should validate URL format before network calls."""
    from labtether.config_flow import LabTetherConfigFlow

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.hass.config_entries.async_entries.return_value = []

    result = await flow.async_step_user({
        CONF_HOST: "lab.local:8443",
        CONF_API_KEY: "token",
    })

    assert result["type"] == "form"
    assert result["errors"]["base"] == "invalid_url"


@pytest.mark.asyncio
async def test_config_flow_advances_to_import_options_on_success():
    """Successful connection should proceed to the import-options preview step."""
    from labtether.config_flow import LabTetherConfigFlow

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.hass.config_entries.async_entries.return_value = []

    with patch("labtether.config_flow.async_get_clientsession", return_value=MagicMock()), \
         patch("labtether.config_flow.LabTetherApiClient.async_get_setup_preview", new=AsyncMock(return_value={
             "host_label": "lab.local:8443",
             "asset_count": 12,
             "telemetry_asset_count": 8,
             "switchable_asset_count": 4,
             "alerts_count": 2,
             "sources_label": "proxmox, docker",
         })):
        result = await flow.async_step_user({
            CONF_HOST: "https://lab.local:8443/",
            CONF_API_KEY: "token",
            CONF_NAME: "My Lab",
            CONF_IGNORE_CERT_ERRORS: True,
        })

    assert result["type"] == "form"
    assert result["step_id"] == "import_options"
    assert result["description_placeholders"]["asset_count"] == "12"
    assert flow._pending_data[CONF_HOST] == "https://lab.local:8443"
    assert flow._pending_data[CONF_IGNORE_CERT_ERRORS] is True


@pytest.mark.asyncio
async def test_config_flow_creates_entry_after_review():
    """Full flow should create an entry with data and options."""
    from labtether.config_flow import LabTetherConfigFlow

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.hass.config_entries.async_entries.return_value = []

    with patch("labtether.config_flow.async_get_clientsession", return_value=MagicMock()), \
         patch("labtether.config_flow.LabTetherApiClient.async_get_setup_preview", new=AsyncMock(return_value={
             "host_label": "lab.local:8443",
             "asset_count": 9,
             "telemetry_asset_count": 6,
             "switchable_asset_count": 3,
             "alerts_count": 1,
             "sources_label": "proxmox",
         })):
        await flow.async_step_user({
            CONF_HOST: "https://lab.local:8443",
            CONF_API_KEY: "token",
            CONF_NAME: "My Lab",
        })

    import_result = await flow.async_step_import_options({
        CONF_IMPORT_BINARY_SENSORS: True,
        CONF_IMPORT_SENSORS: True,
        CONF_IMPORT_SWITCHES: False,
        CONF_ENABLE_RUN_ACTION_SERVICE: True,
        CONF_SCAN_INTERVAL: 45,
    })
    assert import_result["step_id"] == "review"

    review_result = await flow.async_step_review({})

    assert review_result["type"] == "create_entry"
    assert review_result["title"] == "My Lab"
    assert review_result["data"][CONF_HOST] == "https://lab.local:8443"
    assert review_result["options"][CONF_SCAN_INTERVAL] == 45
    assert review_result["options"][CONF_IMPORT_SWITCHES] is False
    assert review_result["options"][CONF_IGNORE_CERT_ERRORS] is False


@pytest.mark.asyncio
async def test_config_flow_surfaces_auth_failures():
    """Authentication failures should map to invalid_auth."""
    from labtether.config_flow import LabTetherConfigFlow
    from labtether.api import LabTetherApiError

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.hass.config_entries.async_entries.return_value = []

    with patch("labtether.config_flow.async_get_clientsession", return_value=MagicMock()), \
         patch("labtether.config_flow.LabTetherApiClient.async_get_setup_preview", new=AsyncMock(side_effect=LabTetherApiError("Authentication failed"))):
        result = await flow.async_step_user({
            CONF_HOST: "https://lab.local:8443",
            CONF_API_KEY: "bad-token",
        })

    assert result["type"] == "form"
    assert result["errors"]["base"] == "invalid_auth"


def test_options_flow_imports():
    """Options flow should be available for the integration."""
    from labtether.config_flow import LabTetherOptionsFlow

    flow = LabTetherOptionsFlow()
    assert flow is not None


@pytest.mark.asyncio
async def test_config_flow_aborts_when_host_already_configured():
    """Duplicate hub URLs should abort instead of creating a second entry."""
    from labtether.config_flow import LabTetherConfigFlow

    existing_entry = MagicMock()
    existing_entry.data = {CONF_HOST: "https://lab.local:8443"}
    existing_entry.entry_id = "entry-1"

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.hass.config_entries.async_entries.return_value = [existing_entry]

    with patch("labtether.config_flow.async_get_clientsession", return_value=MagicMock()), \
         patch("labtether.config_flow.LabTetherApiClient.async_get_setup_preview", new=AsyncMock(return_value={
             "host_label": "lab.local:8443",
             "asset_count": 1,
             "telemetry_asset_count": 1,
             "switchable_asset_count": 0,
             "alerts_count": 0,
             "sources_label": "proxmox",
         })):
        result = await flow.async_step_user({
            CONF_HOST: "https://lab.local:8443/",
            CONF_API_KEY: "token",
        })

    assert result["type"] == "abort"
    assert result["reason"] == "already_configured"


@pytest.mark.asyncio
async def test_reconfigure_updates_existing_entry():
    """Reconfigure should update the stored host and title."""
    from labtether.config_flow import LabTetherConfigFlow

    entry = MagicMock()
    entry.entry_id = "entry-1"
    entry.data = {
        CONF_HOST: "https://old.local:8443",
        CONF_API_KEY: "old-token",
        CONF_NAME: "Old Lab",
        CONF_IGNORE_CERT_ERRORS: False,
    }
    entry.options = {}

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.context = {"entry_id": "entry-1"}
    flow.hass.config_entries.async_get_entry.return_value = entry
    flow.hass.config_entries.async_entries.return_value = [entry]
    flow.hass.config_entries.async_schedule_reload = MagicMock()

    with patch("labtether.config_flow.async_get_clientsession", return_value=MagicMock()), \
         patch("labtether.config_flow.LabTetherApiClient.async_get_setup_preview", new=AsyncMock(return_value={
             "host_label": "new.local:8443",
             "asset_count": 4,
             "telemetry_asset_count": 2,
             "switchable_asset_count": 1,
             "alerts_count": 0,
             "sources_label": "docker",
         })):
        result = await flow.async_step_reconfigure({
            CONF_HOST: "https://new.local:8443",
            CONF_API_KEY: "new-token",
            CONF_NAME: "New Lab",
            CONF_IGNORE_CERT_ERRORS: True,
        })

    assert result["type"] == "abort"
    assert result["reason"] == "reconfigure_successful"
    flow.hass.config_entries.async_update_entry.assert_called_once()
    update_kwargs = flow.hass.config_entries.async_update_entry.call_args.kwargs
    assert update_kwargs["options"][CONF_IGNORE_CERT_ERRORS] is True
    flow.hass.config_entries.async_schedule_reload.assert_called_once_with("entry-1")


@pytest.mark.asyncio
async def test_reauth_updates_existing_api_key():
    """Reauth should update the API key and reload the entry."""
    from labtether.config_flow import LabTetherConfigFlow

    entry = MagicMock()
    entry.entry_id = "entry-1"
    entry.data = {
        CONF_HOST: "https://lab.local:8443",
        CONF_API_KEY: "old-token",
        CONF_NAME: "My Lab",
        CONF_IGNORE_CERT_ERRORS: False,
    }
    entry.options = {}

    flow = LabTetherConfigFlow()
    flow.hass = MagicMock()
    flow.context = {"entry_id": "entry-1"}
    flow.hass.config_entries.async_get_entry.return_value = entry
    flow.hass.config_entries.async_schedule_reload = MagicMock()

    await flow.async_step_reauth({})

    with patch("labtether.config_flow.async_get_clientsession", return_value=MagicMock()), \
         patch("labtether.config_flow.LabTetherApiClient.async_get_setup_preview", new=AsyncMock(return_value={
             "host_label": "lab.local:8443",
             "asset_count": 4,
             "telemetry_asset_count": 2,
             "switchable_asset_count": 1,
             "alerts_count": 0,
             "sources_label": "docker",
         })):
        result = await flow.async_step_reauth_confirm({
            CONF_API_KEY: "fresh-token",
            CONF_IGNORE_CERT_ERRORS: True,
        })

    assert result["type"] == "abort"
    assert result["reason"] == "reauth_successful"
    flow.hass.config_entries.async_update_entry.assert_called_once()
    update_kwargs = flow.hass.config_entries.async_update_entry.call_args.kwargs
    assert update_kwargs["options"][CONF_IGNORE_CERT_ERRORS] is True
    flow.hass.config_entries.async_schedule_reload.assert_called_once_with("entry-1")
