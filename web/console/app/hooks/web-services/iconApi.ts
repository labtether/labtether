import type { ServiceCustomIcon, ServiceCustomIconInput, ServiceCustomIconRenameInput } from "./types";
import { asObject, asString } from "./payloadValues";
import { safeJSON } from "./serviceRequest";

export async function listCustomServiceIcons() {
  const res = await fetch("/api/services/web/icon-library", {
    cache: "no-store",
  });
  if (!res.ok) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
  const data = (await safeJSON(res)) as { icons?: unknown } | null;
  return normalizeCustomServiceIconList(data?.icons);
}

export async function createCustomServiceIcon(input: ServiceCustomIconInput) {
  const res = await fetch("/api/services/web/icon-library", {
    method: "POST",
    cache: "no-store",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input),
  });
  const payload = (await safeJSON(res)) as { icon?: unknown; error?: string } | null;
  if (!res.ok) {
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
  const icon = normalizeCustomServiceIcon(payload?.icon);
  if (!icon) {
    throw new Error("service icon create response missing icon");
  }
  return icon;
}

export async function deleteCustomServiceIcon(id: string) {
  const params = new URLSearchParams();
  params.set("id", id);
  const res = await fetch(`/api/services/web/icon-library?${params.toString()}`, {
    method: "DELETE",
    cache: "no-store",
  });
  if (!res.ok && res.status !== 204) {
    const payload = (await safeJSON(res)) as { error?: string } | null;
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
}

export async function renameCustomServiceIcon(input: ServiceCustomIconRenameInput) {
  const params = new URLSearchParams();
  params.set("id", input.id);
  const res = await fetch(`/api/services/web/icon-library?${params.toString()}`, {
    method: "PATCH",
    cache: "no-store",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ name: input.name }),
  });
  const payload = (await safeJSON(res)) as { icon?: unknown; error?: string } | null;
  if (!res.ok) {
    throw new Error(payload?.error ?? `HTTP ${res.status}`);
  }
  const icon = normalizeCustomServiceIcon(payload?.icon);
  if (!icon) {
    throw new Error("service icon rename response missing icon");
  }
  return icon;
}

function normalizeCustomServiceIcon(value: unknown): ServiceCustomIcon | null {
  const raw = asObject(value);
  if (!raw) {
    return null;
  }
  const id = asString(raw.id);
  const name = asString(raw.name);
  if (!id && !name) {
    return null;
  }
  return {
    id,
    name,
    data_url: asString(raw.data_url),
    created_at: asString(raw.created_at) || undefined,
    updated_at: asString(raw.updated_at) || undefined,
  };
}

function normalizeCustomServiceIconList(value: unknown): ServiceCustomIcon[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map(normalizeCustomServiceIcon)
    .filter((entry): entry is ServiceCustomIcon => entry !== null);
}
