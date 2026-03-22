"""LabTether API client."""

from __future__ import annotations

import logging
from typing import Any
from urllib.parse import urlparse

from aiohttp import ClientSession, ClientError

from .const import (
    API_ASSETS,
    API_METRICS_OVERVIEW,
    API_ALERTS_INSTANCES,
    API_ACTIONS_EXECUTE,
    EXCLUDED_SOURCE,
    TELEMETRY_KINDS,
    CONTROLLABLE_KINDS,
)

_LOGGER = logging.getLogger(__name__)


class LabTetherApiError(Exception):
    """Exception for LabTether API errors."""


class LabTetherApiClient:
    """Client to interact with the LabTether REST API."""

    def __init__(
        self,
        host: str,
        api_key: str,
        session: ClientSession,
        ignore_cert_errors: bool = False,
    ) -> None:
        self._host = host.rstrip("/")
        self._api_key = api_key
        self._session = session
        self._ignore_cert_errors = ignore_cert_errors

    @property
    def host(self) -> str:
        """Return the hub host URL."""
        return self._host

    @property
    def _headers(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self._api_key}"}

    @property
    def _request_kwargs(self) -> dict[str, Any]:
        if self._ignore_cert_errors:
            return {"ssl": False}
        return {}

    async def _request(self, method: str, path: str, **kwargs: Any) -> Any:
        """Make an authenticated request."""
        url = f"{self._host}{path}"
        request_kwargs = {
            "headers": self._headers,
            **kwargs,
            **self._request_kwargs,
        }
        try:
            async with self._session.request(method, url, **request_kwargs) as resp:
                if resp.status == 401:
                    raise LabTetherApiError("Authentication failed")
                if resp.status >= 400:
                    text = await resp.text()
                    raise LabTetherApiError(f"API error {resp.status}: {text}")
                if resp.content_type == "application/json":
                    return await resp.json()
                return {}
        except ClientError as err:
            raise LabTetherApiError(f"Connection error: {err}") from err

    async def _get(self, path: str, params: dict | None = None) -> Any:
        """Make an authenticated GET request."""
        return await self._request("GET", path, params=params)

    async def _post(self, path: str, data: dict) -> Any:
        """Make an authenticated POST request."""
        return await self._request("POST", path, json=data)

    async def async_validate_connection(self) -> bool:
        """Validate the connection to LabTether hub."""
        url = f"{self._host}{API_ASSETS}"
        try:
            async with self._session.get(url, headers=self._headers, **self._request_kwargs) as resp:
                return resp.status == 200
        except ClientError:
            return False

    async def async_get_setup_preview(self) -> dict[str, Any]:
        """Return a setup preview summary."""
        parsed = urlparse(self._host)
        assets = await self.async_get_assets()
        metrics = await self.async_get_metrics_overview()
        alerts_count = await self.async_get_firing_alerts_count()
        telemetry_assets = [asset for asset in assets if asset.get("type") in TELEMETRY_KINDS]
        switchable_assets = [asset for asset in assets if asset.get("type") in CONTROLLABLE_KINDS]
        sources = sorted({str(asset.get("source", "unknown")) for asset in assets})[:5]
        return {
            "host_label": parsed.netloc or self._host,
            "asset_count": len(assets),
            "telemetry_asset_count": len(telemetry_assets),
            "metric_asset_count": len(metrics),
            "switchable_asset_count": len(switchable_assets),
            "alerts_count": alerts_count,
            "sources_label": ", ".join(sources) if sources else "none",
        }

    async def async_get_assets(self) -> list[dict]:
        """Get all assets, excluding HA-sourced ones to prevent circular mirroring."""
        data = await self._get(API_ASSETS)
        assets = data.get("assets", [])
        return [
            asset
            for asset in assets
            if asset.get("source") != EXCLUDED_SOURCE and asset.get("id")
        ]

    async def async_get_metrics_overview(self) -> dict[str, dict]:
        """Get latest metrics for all assets, keyed by asset_id."""
        data = await self._get(API_METRICS_OVERVIEW)
        result = {}
        for entry in data.get("assets", []):
            asset_id = entry.get("asset_id")
            if asset_id:
                result[asset_id] = entry.get("metrics", {})
        return result

    async def async_get_firing_alerts_count(self) -> int:
        """Get the count of currently firing alerts."""
        data = await self._get(API_ALERTS_INSTANCES, params={"status": "firing"})
        return len(data.get("instances", []))

    async def async_run_action(
        self,
        asset_id: str,
        action_type: str = "connector_action",
        connector_id: str | None = None,
        action_id: str | None = None,
        params: dict | None = None,
    ) -> dict:
        """Run an action on a LabTether asset."""
        body: dict[str, Any] = {"type": action_type, "asset_id": asset_id}
        if connector_id:
            body["connector_id"] = connector_id
        if action_id:
            body["action_id"] = action_id
        if params:
            body["params"] = params
        return await self._post(API_ACTIONS_EXECUTE, body)
