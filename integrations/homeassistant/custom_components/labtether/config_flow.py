"""Config flow for LabTether integration."""

from __future__ import annotations

import logging
from urllib.parse import urlparse

import voluptuous as vol

from homeassistant import config_entries
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .api import LabTetherApiClient, LabTetherApiError
from .const import (
    DOMAIN,
    CONF_HOST,
    CONF_API_KEY,
    CONF_NAME,
    CONF_IGNORE_CERT_ERRORS,
    CONF_IMPORT_BINARY_SENSORS,
    CONF_IMPORT_SENSORS,
    CONF_IMPORT_SWITCHES,
    CONF_ENABLE_RUN_ACTION_SERVICE,
    CONF_SCAN_INTERVAL,
    DEFAULT_IMPORT_BINARY_SENSORS,
    DEFAULT_IMPORT_SENSORS,
    DEFAULT_IMPORT_SWITCHES,
    DEFAULT_ENABLE_RUN_ACTION_SERVICE,
    DEFAULT_SCAN_INTERVAL,
)

_LOGGER = logging.getLogger(__name__)


def _connection_schema(defaults: dict | None = None) -> vol.Schema:
    defaults = defaults or {}
    return vol.Schema(
        {
            vol.Required(CONF_HOST, default=defaults.get(CONF_HOST, "")): str,
            vol.Required(CONF_API_KEY, default=defaults.get(CONF_API_KEY, "")): str,
            vol.Optional(CONF_NAME, default=defaults.get(CONF_NAME, "")): str,
            vol.Optional(CONF_IGNORE_CERT_ERRORS, default=bool(defaults.get(CONF_IGNORE_CERT_ERRORS, False))): bool,
        }
    )


USER_DATA_SCHEMA = _connection_schema()


IMPORT_OPTIONS_SCHEMA = vol.Schema(
    {
        vol.Optional(CONF_IMPORT_BINARY_SENSORS, default=DEFAULT_IMPORT_BINARY_SENSORS): bool,
        vol.Optional(CONF_IMPORT_SENSORS, default=DEFAULT_IMPORT_SENSORS): bool,
        vol.Optional(CONF_IMPORT_SWITCHES, default=DEFAULT_IMPORT_SWITCHES): bool,
        vol.Optional(CONF_ENABLE_RUN_ACTION_SERVICE, default=DEFAULT_ENABLE_RUN_ACTION_SERVICE): bool,
        vol.Optional(CONF_SCAN_INTERVAL, default=DEFAULT_SCAN_INTERVAL): vol.All(vol.Coerce(int), vol.Range(min=5, max=3600)),
    }
)


class LabTetherConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    """Handle a config flow for LabTether."""

    VERSION = 1

    def __init__(self) -> None:
        self._pending_data: dict | None = None
        self._pending_options: dict | None = None
        self._preview: dict | None = None

    @staticmethod
    def _normalize_host(host: str) -> str:
        return host.strip().rstrip("/")

    @staticmethod
    def _host_is_valid(host: str) -> bool:
        parsed = urlparse(host)
        return parsed.scheme in {"http", "https"} and bool(parsed.netloc)

    @staticmethod
    def _default_title(data: dict, preview: dict | None) -> str:
        if data.get(CONF_NAME):
            return str(data[CONF_NAME]).strip()
        if preview and preview.get("host_label"):
            return f"LabTether ({preview['host_label']})"
        return "LabTether"

    def _build_preview_placeholders(self) -> dict[str, str]:
        preview = self._preview or {}
        return {
            "host_label": str(preview.get("host_label", "unknown")),
            "asset_count": str(preview.get("asset_count", 0)),
            "telemetry_asset_count": str(preview.get("telemetry_asset_count", 0)),
            "switchable_asset_count": str(preview.get("switchable_asset_count", 0)),
            "alerts_count": str(preview.get("alerts_count", 0)),
            "sources_label": str(preview.get("sources_label", "none")),
        }

    def _build_review_placeholders(self) -> dict[str, str]:
        pending_data = self._pending_data or {}
        pending_options = self._pending_options or {}
        preview = self._preview or {}
        return {
            "title": self._default_title(pending_data, preview),
            "host": str(pending_data.get(CONF_HOST, "")),
            "ignore_cert_errors": "Enabled" if pending_data.get(CONF_IGNORE_CERT_ERRORS) else "Disabled",
            "import_status_entities": "Yes" if pending_options.get(CONF_IMPORT_BINARY_SENSORS, DEFAULT_IMPORT_BINARY_SENSORS) else "No",
            "import_telemetry_sensors": "Yes" if pending_options.get(CONF_IMPORT_SENSORS, DEFAULT_IMPORT_SENSORS) else "No",
            "import_power_switches": "Yes" if pending_options.get(CONF_IMPORT_SWITCHES, DEFAULT_IMPORT_SWITCHES) else "No",
            "enable_run_action_service": "Yes" if pending_options.get(CONF_ENABLE_RUN_ACTION_SERVICE, DEFAULT_ENABLE_RUN_ACTION_SERVICE) else "No",
            "scan_interval": str(pending_options.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)),
            "asset_count": str(preview.get("asset_count", 0)),
            "alerts_count": str(preview.get("alerts_count", 0)),
        }

    @staticmethod
    def _options_with_tls_pref(entry, ignore_cert_errors: bool) -> dict:
        options = dict(entry.options)
        options[CONF_IGNORE_CERT_ERRORS] = ignore_cert_errors
        return options

    def _current_entries(self):
        return self._async_current_entries()

    def _entry_for_host(self, host: str, ignore_entry_id: str | None = None):
        normalized_host = self._normalize_host(host)
        for entry in self._current_entries():
            if ignore_entry_id is not None and getattr(entry, "entry_id", None) == ignore_entry_id:
                continue
            if self._normalize_host(entry.data.get(CONF_HOST, "")) == normalized_host:
                return entry
        return None

    @staticmethod
    def _classify_api_error(err: LabTetherApiError) -> str:
        message = str(err).lower()
        if "authentication failed" in message:
            return "invalid_auth"
        if "ssl" in message or "certificate" in message:
            return "tls_error"
        return "cannot_connect"

    async def _async_preview_connection(self, user_input: dict) -> tuple[dict | None, str | None]:
        normalized_host = self._normalize_host(user_input[CONF_HOST])
        if not self._host_is_valid(normalized_host):
            return None, "invalid_url"

        try:
            session = async_get_clientsession(self.hass)
            client = LabTetherApiClient(
                host=normalized_host,
                api_key=user_input[CONF_API_KEY],
                session=session,
                ignore_cert_errors=bool(user_input.get(CONF_IGNORE_CERT_ERRORS)),
            )
            preview = await client.async_get_setup_preview()
            data = {
                CONF_HOST: normalized_host,
                CONF_API_KEY: user_input[CONF_API_KEY],
                CONF_NAME: user_input.get(CONF_NAME, "").strip(),
                CONF_IGNORE_CERT_ERRORS: bool(user_input.get(CONF_IGNORE_CERT_ERRORS, False)),
            }
            return {"data": data, "preview": preview}, None
        except LabTetherApiError as err:
            return None, self._classify_api_error(err)
        except Exception:  # noqa: BLE001
            _LOGGER.exception("Unexpected exception")
            return None, "unknown"

    async def async_step_user(self, user_input=None):
        """Handle the connection step."""
        errors = {}

        if user_input is not None:
            preview_result, error = await self._async_preview_connection(user_input)
            if error:
                errors["base"] = error
            else:
                data = preview_result["data"]
                if self._entry_for_host(data[CONF_HOST]) is not None:
                    return self.async_abort(reason="already_configured")
                self._pending_data = data
                self._preview = preview_result["preview"]
                return await self.async_step_import_options()

        return self.async_show_form(
            step_id="user",
            data_schema=_connection_schema(),
            errors=errors,
        )

    async def async_step_import_options(self, user_input=None):
        """Choose which Home Assistant surfaces to create."""
        if self._pending_data is None or self._preview is None:
            return await self.async_step_user()

        if user_input is not None:
            self._pending_options = {
                CONF_IMPORT_BINARY_SENSORS: bool(user_input.get(CONF_IMPORT_BINARY_SENSORS, DEFAULT_IMPORT_BINARY_SENSORS)),
                CONF_IMPORT_SENSORS: bool(user_input.get(CONF_IMPORT_SENSORS, DEFAULT_IMPORT_SENSORS)),
                CONF_IMPORT_SWITCHES: bool(user_input.get(CONF_IMPORT_SWITCHES, DEFAULT_IMPORT_SWITCHES)),
                CONF_ENABLE_RUN_ACTION_SERVICE: bool(user_input.get(CONF_ENABLE_RUN_ACTION_SERVICE, DEFAULT_ENABLE_RUN_ACTION_SERVICE)),
                CONF_SCAN_INTERVAL: int(user_input.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)),
            }
            return await self.async_step_review()

        return self.async_show_form(
            step_id="import_options",
            data_schema=IMPORT_OPTIONS_SCHEMA,
            description_placeholders=self._build_preview_placeholders(),
        )

    async def async_step_review(self, user_input=None):
        """Review the final setup choices."""
        if self._pending_data is None or self._pending_options is None:
            return await self.async_step_user()

        if user_input is not None:
            options = dict(self._pending_options)
            options[CONF_IGNORE_CERT_ERRORS] = bool(
                self._pending_data.get(CONF_IGNORE_CERT_ERRORS, False)
            )
            return self.async_create_entry(
                title=self._default_title(self._pending_data, self._preview),
                data=self._pending_data,
                options=options,
            )

        return self.async_show_form(
            step_id="review",
            data_schema=vol.Schema({}),
            description_placeholders=self._build_review_placeholders(),
        )

    async def async_step_reconfigure(self, user_input=None):
        """Allow updating connection details for an existing entry."""
        errors = {}
        try:
            entry = self._get_reconfigure_entry()
        except Exception:  # noqa: BLE001
            return self.async_abort(reason="unknown")

        if user_input is not None:
            preview_result, error = await self._async_preview_connection(user_input)
            if error:
                errors["base"] = error
            else:
                data = preview_result["data"]
                if self._entry_for_host(data[CONF_HOST], ignore_entry_id=entry.entry_id) is not None:
                    return self.async_abort(reason="already_configured")
                return self.async_update_reload_and_abort(
                    entry,
                    title=self._default_title(data, preview_result["preview"]),
                    data=data,
                    options=self._options_with_tls_pref(
                        entry, data[CONF_IGNORE_CERT_ERRORS]
                    ),
                    reason="reconfigure_successful",
                )

        defaults = {
            CONF_HOST: entry.data.get(CONF_HOST, ""),
            CONF_API_KEY: entry.data.get(CONF_API_KEY, ""),
            CONF_NAME: entry.data.get(CONF_NAME, ""),
            CONF_IGNORE_CERT_ERRORS: bool(
                entry.options.get(
                    CONF_IGNORE_CERT_ERRORS,
                    entry.data.get(CONF_IGNORE_CERT_ERRORS, False),
                )
            ),
        }
        return self.async_show_form(
            step_id="reconfigure",
            data_schema=_connection_schema(defaults),
            errors=errors,
        )

    async def async_step_reauth(self, _entry_data):
        """Begin reauthentication flow."""
        try:
            self._get_reauth_entry()
        except Exception:  # noqa: BLE001
            return self.async_abort(reason="unknown")
        return await self.async_step_reauth_confirm()

    async def async_step_reauth_confirm(self, user_input=None):
        """Confirm new credentials for a broken connection."""
        errors = {}
        try:
            entry = self._get_reauth_entry()
        except Exception:  # noqa: BLE001
            return self.async_abort(reason="unknown")

        if user_input is not None:
            reauth_data = {
                CONF_HOST: entry.data.get(CONF_HOST, ""),
                CONF_API_KEY: user_input[CONF_API_KEY],
                CONF_NAME: entry.data.get(CONF_NAME, ""),
                CONF_IGNORE_CERT_ERRORS: bool(user_input.get(CONF_IGNORE_CERT_ERRORS, False)),
            }
            preview_result, error = await self._async_preview_connection(reauth_data)
            if error:
                errors["base"] = error
            else:
                return self.async_update_reload_and_abort(
                    entry,
                    data_updates={
                        CONF_API_KEY: reauth_data[CONF_API_KEY],
                        CONF_IGNORE_CERT_ERRORS: reauth_data[CONF_IGNORE_CERT_ERRORS],
                    },
                    options=self._options_with_tls_pref(
                        entry, reauth_data[CONF_IGNORE_CERT_ERRORS]
                    ),
                    reason="reauth_successful",
                )

        return self.async_show_form(
            step_id="reauth_confirm",
            data_schema=vol.Schema(
                {
                    vol.Required(CONF_API_KEY): str,
                    vol.Optional(
                        CONF_IGNORE_CERT_ERRORS,
                        default=bool(
                            entry.options.get(
                                CONF_IGNORE_CERT_ERRORS,
                                entry.data.get(CONF_IGNORE_CERT_ERRORS, False),
                            )
                        ),
                    ): bool,
                }
            ),
            errors=errors,
            description_placeholders={"host": str(entry.data.get(CONF_HOST, ""))},
        )

    @staticmethod
    def async_get_options_flow(config_entry):
        """Return the options flow."""
        return LabTetherOptionsFlow()


class LabTetherOptionsFlow(config_entries.OptionsFlow):
    """Handle LabTether integration options."""

    async def async_step_init(self, user_input=None):
        """Manage integration options."""
        if user_input is not None:
            return self.async_create_entry(title="", data=user_input)

        options_schema = vol.Schema(
            {
                vol.Optional(
                    CONF_IGNORE_CERT_ERRORS,
                    default=bool(self.config_entry.options.get(CONF_IGNORE_CERT_ERRORS, self.config_entry.data.get(CONF_IGNORE_CERT_ERRORS, False))),
                ): bool,
                vol.Optional(
                    CONF_IMPORT_BINARY_SENSORS,
                    default=bool(self.config_entry.options.get(CONF_IMPORT_BINARY_SENSORS, self.config_entry.data.get(CONF_IMPORT_BINARY_SENSORS, DEFAULT_IMPORT_BINARY_SENSORS))),
                ): bool,
                vol.Optional(
                    CONF_IMPORT_SENSORS,
                    default=bool(self.config_entry.options.get(CONF_IMPORT_SENSORS, self.config_entry.data.get(CONF_IMPORT_SENSORS, DEFAULT_IMPORT_SENSORS))),
                ): bool,
                vol.Optional(
                    CONF_IMPORT_SWITCHES,
                    default=bool(self.config_entry.options.get(CONF_IMPORT_SWITCHES, self.config_entry.data.get(CONF_IMPORT_SWITCHES, DEFAULT_IMPORT_SWITCHES))),
                ): bool,
                vol.Optional(
                    CONF_ENABLE_RUN_ACTION_SERVICE,
                    default=bool(
                        self.config_entry.options.get(
                            CONF_ENABLE_RUN_ACTION_SERVICE,
                            self.config_entry.data.get(CONF_ENABLE_RUN_ACTION_SERVICE, DEFAULT_ENABLE_RUN_ACTION_SERVICE),
                        )
                    ),
                ): bool,
                vol.Optional(
                    CONF_SCAN_INTERVAL,
                    default=int(self.config_entry.options.get(CONF_SCAN_INTERVAL, self.config_entry.data.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL))),
                ): vol.All(vol.Coerce(int), vol.Range(min=5, max=3600)),
            }
        )
        return self.async_show_form(step_id="init", data_schema=options_schema)
