import type { ManualWebServiceInput, WebServiceOverrideInput, WebServiceOverride } from "./types";
import { asObject, asString, asBoolean } from "./payloadValues";
import { safeJSON } from "./serviceRequest";

export async function createManualService(input: ManualWebServiceInput) {
  const res = await fetch("/api/services/web/manual", {
    method: "POST",
    cache: "no-store",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
}

export async function updateManualService(id: string, patch: Partial<ManualWebServiceInput>) {
  const res = await fetch(`/api/services/web/manual/${encodeURIComponent(id)}`, {
    method: "PATCH",
    cache: "no-store",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(patch),
  });
  if (!res.ok) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
}

export async function deleteManualService(id: string) {
  const res = await fetch(`/api/services/web/manual/${encodeURIComponent(id)}`, {
    method: "DELETE",
    cache: "no-store",
  });
  if (!res.ok && res.status !== 204) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
}

export async function saveServiceOverride(input: WebServiceOverrideInput) {
  const res = await fetch("/api/services/web/overrides", {
    method: "POST",
    cache: "no-store",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
}

export async function listServiceOverrides(hostAssetID?: string) {
  const params = new URLSearchParams();
  if (hostAssetID) {
    params.set("host", hostAssetID);
  }
  const url = `/api/services/web/overrides${params.toString() ? "?" + params.toString() : ""}`;
  const res = await fetch(url, {
    cache: "no-store",
  });
  if (!res.ok) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
  const data = (await safeJSON(res)) as { overrides?: WebServiceOverride[] } | null;
  return normalizeWebServiceOverrideList(data?.overrides);
}

export async function deleteServiceOverride(hostAssetID: string, serviceID: string) {
  const params = new URLSearchParams();
  if (hostAssetID) {
    params.set("host", hostAssetID);
  }
  if (serviceID) {
    params.set("service_id", serviceID);
  }
  const url = `/api/services/web/overrides${params.toString() ? "?" + params.toString() : ""}`;
  const res = await fetch(url, {
    method: "DELETE",
    cache: "no-store",
  });
  if (!res.ok && res.status !== 204) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
}

function normalizeWebServiceOverride(value: unknown): WebServiceOverride | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const hostAssetID = asString(raw.host_asset_id);
  const serviceID = asString(raw.service_id);
  if (!hostAssetID && !serviceID) {
    return null;
  }
  return {
    host_asset_id: hostAssetID,
    service_id: serviceID,
    name_override: asString(raw.name_override) || undefined,
    category_override: asString(raw.category_override) || undefined,
    url_override: asString(raw.url_override) || undefined,
    icon_key_override: asString(raw.icon_key_override) || undefined,
    tags_override: asString(raw.tags_override) || undefined,
    hidden: asBoolean(raw.hidden),
    updated_at: asString(raw.updated_at) || undefined,
  };
}

function normalizeWebServiceOverrideList(value: unknown): WebServiceOverride[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeWebServiceOverride)
    .filter((entry): entry is WebServiceOverride => entry !== null);
}
