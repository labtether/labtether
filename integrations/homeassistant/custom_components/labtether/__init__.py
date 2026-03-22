"""The LabTether integration."""

from __future__ import annotations

import logging

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.helpers import config_validation as cv, device_registry as dr, entity_registry as er
from homeassistant.helpers.aiohttp_client import async_get_clientsession
import voluptuous as vol

from .api import LabTetherApiClient
from .const import (
    DOMAIN,
    CONF_HOST,
    CONF_API_KEY,
    CONF_IGNORE_CERT_ERRORS,
    CONF_ENABLE_RUN_ACTION_SERVICE,
    CONF_SCAN_INTERVAL,
    DEFAULT_ENABLE_RUN_ACTION_SERVICE,
    DEFAULT_SCAN_INTERVAL,
    PLATFORMS,
    entry_pref,
    TELEMETRY_KINDS,
    CONTROLLABLE_KINDS,
    POWER_ACTION_SOURCES,
    hub_registry_key,
    asset_registry_key,
)
from .coordinator import LabTetherCoordinator

_LOGGER = logging.getLogger(__name__)

SERVICE_RUN_ACTION = "run_action"
SERVICE_RUN_ACTION_SCHEMA = vol.Schema(
    {
        vol.Required("asset_id"): cv.string,
        vol.Required("action"): cv.string,
        vol.Optional("connector_id"): cv.string,
        vol.Optional("params"): dict,
    }
)


def _run_action_enabled(entry: ConfigEntry) -> bool:
    return bool(entry_pref(entry, CONF_ENABLE_RUN_ACTION_SERVICE, DEFAULT_ENABLE_RUN_ACTION_SERVICE))


def _build_client(hass: HomeAssistant, entry: ConfigEntry) -> LabTetherApiClient:
    session = async_get_clientsession(hass)
    return LabTetherApiClient(
        host=entry.data[CONF_HOST],
        api_key=entry.data[CONF_API_KEY],
        session=session,
        ignore_cert_errors=bool(entry_pref(entry, CONF_IGNORE_CERT_ERRORS, False)),
    )


def _ensure_hub_device(hass: HomeAssistant, entry: ConfigEntry, client: LabTetherApiClient) -> None:
    """Ensure the hub device exists even if hub entities are disabled."""
    device_registry = dr.async_get(hass)
    device_registry.async_get_or_create(
        config_entry_id=entry.entry_id,
        identifiers={(DOMAIN, hub_registry_key(entry.entry_id))},
        name="LabTether Hub",
        manufacturer="LabTether",
        model="Hub",
        configuration_url=client.host,
    )


def _select_service_target(hass: HomeAssistant, asset_id: str):
    """Find the loaded entry that owns the requested asset."""
    for entry_data in hass.data.get(DOMAIN, {}).values():
        coordinator = entry_data["coordinator"]
        if coordinator.data.get_asset(asset_id) is not None:
            return entry_data
    return None


def _legacy_unique_id_migrations(coordinator: LabTetherCoordinator) -> dict[tuple[str, str], str]:
    """Build entity-registry unique-id migrations for older global IDs."""
    entry_id = coordinator.entry_id
    migrations = {
        ("binary_sensor", f"{DOMAIN}_hub_status"): f"{DOMAIN}_{entry_id}_hub_status",
        ("sensor", f"{DOMAIN}_hub_total_assets"): f"{DOMAIN}_{entry_id}_hub_total_assets",
        ("sensor", f"{DOMAIN}_hub_active_alerts"): f"{DOMAIN}_{entry_id}_hub_active_alerts",
    }

    for asset in coordinator.data.assets:
        asset_id = asset.get("id")
        if not asset_id:
            continue

        migrations[("binary_sensor", f"{DOMAIN}_{asset_id}_status")] = (
            f"{DOMAIN}_{entry_id}_{asset_id}_status"
        )

        if asset.get("type") in TELEMETRY_KINDS:
            for metric_key in ("cpu_used_percent", "memory_used_percent", "disk_used_percent"):
                migrations[("sensor", f"{DOMAIN}_{asset_id}_{metric_key}")] = (
                    f"{DOMAIN}_{entry_id}_{asset_id}_{metric_key}"
                )

        if (
            asset.get("type") in CONTROLLABLE_KINDS
            and asset.get("source", "") in POWER_ACTION_SOURCES
        ):
            migrations[("switch", f"{DOMAIN}_{asset_id}_power")] = (
                f"{DOMAIN}_{entry_id}_{asset_id}_power"
            )

    return migrations


def _migrate_entity_unique_ids(hass: HomeAssistant, entry: ConfigEntry, coordinator: LabTetherCoordinator) -> None:
    """Migrate older global entity unique IDs to per-entry namespaced IDs."""
    entity_registry = er.async_get(hass)
    migrations = _legacy_unique_id_migrations(coordinator)
    for entity_entry in er.async_entries_for_config_entry(entity_registry, entry.entry_id):
        new_unique_id = migrations.get((entity_entry.platform, entity_entry.unique_id))
        if new_unique_id and new_unique_id != entity_entry.unique_id:
            entity_registry.async_update_entity(
                entity_entry.entity_id, new_unique_id=new_unique_id
            )


def _register_run_action_service(hass: HomeAssistant) -> None:
    if hass.services.has_service(DOMAIN, SERVICE_RUN_ACTION):
        return

    async def handle_run_action(call: ServiceCall) -> None:
        """Handle the run_action service call."""
        asset_id = call.data["asset_id"]
        action = call.data["action"]
        connector_id = call.data.get("connector_id")
        params = call.data.get("params")

        target = _select_service_target(hass, asset_id)
        if target is None:
            raise ValueError(f"No loaded LabTether entry exposes asset_id={asset_id!r}")

        client: LabTetherApiClient = target["client"]
        await client.async_run_action(
            asset_id=asset_id,
            action_type="connector_action",
            connector_id=connector_id,
            action_id=action,
            params=params,
        )

    hass.services.async_register(
        DOMAIN,
        SERVICE_RUN_ACTION,
        handle_run_action,
        schema=SERVICE_RUN_ACTION_SCHEMA,
    )


def _sync_run_action_service(hass: HomeAssistant) -> None:
    entries = hass.data.get(DOMAIN, {})
    if any(_run_action_enabled(entry_data["entry"]) for entry_data in entries.values()):
        _register_run_action_service(hass)
    elif hass.services.has_service(DOMAIN, SERVICE_RUN_ACTION):
        hass.services.async_remove(DOMAIN, SERVICE_RUN_ACTION)


async def _async_reload_entry(hass: HomeAssistant, entry: ConfigEntry) -> None:
    """Reload config entry after options change."""
    await hass.config_entries.async_reload(entry.entry_id)


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Set up LabTether from a config entry."""
    client = _build_client(hass, entry)
    _ensure_hub_device(hass, entry, client)
    coordinator = LabTetherCoordinator(
        hass,
        client,
        entry.entry_id,
        scan_interval_seconds=int(entry_pref(entry, CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)),
    )
    await coordinator.async_config_entry_first_refresh()
    _migrate_entity_unique_ids(hass, entry, coordinator)

    hass.data.setdefault(DOMAIN, {})
    hass.data[DOMAIN][entry.entry_id] = {
        "coordinator": coordinator,
        "client": client,
        "entry": entry,
    }
    entry.async_on_unload(entry.add_update_listener(_async_reload_entry))

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    _sync_run_action_service(hass)

    return True


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Unload a LabTether config entry."""
    unload_ok = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    if unload_ok:
        hass.data[DOMAIN].pop(entry.entry_id, None)
        _sync_run_action_service(hass)
    return unload_ok


async def async_remove_config_entry_device(
    hass: HomeAssistant, entry: ConfigEntry, device_entry: dr.DeviceEntry
) -> bool:
    """Allow manual removal of stale asset devices from the registry."""
    if (DOMAIN, hub_registry_key(entry.entry_id)) in device_entry.identifiers:
        return False

    current_assets = hass.data.get(DOMAIN, {}).get(entry.entry_id, {}).get("coordinator")
    current_asset_ids = set()
    if current_assets is not None:
        current_asset_ids = {
            asset_registry_key(entry.entry_id, asset["id"])
            for asset in current_assets.data.assets
            if "id" in asset
        }

    return not any(
        identifier_domain == DOMAIN and identifier_id in current_asset_ids
        for identifier_domain, identifier_id in device_entry.identifiers
    )
