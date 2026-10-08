export async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function fetchServicePayload({
  detail,
  hostAssetID,
  includeHiddenServices,
  serviceID,
  signal,
}: {
  detail: "compact" | "full";
  hostAssetID?: string;
  includeHiddenServices: boolean;
  serviceID?: string;
  signal?: AbortSignal;
}) {
  const params = new URLSearchParams();
  if (hostAssetID) params.set("host", hostAssetID);
  if (includeHiddenServices) params.set("include_hidden", "true");
  if (detail !== "full") params.set("detail", detail);
  if (serviceID) params.set("service_id", serviceID);
  const url = `/api/services/web${params.toString() ? "?" + params.toString() : ""}`;
  const response = await fetch(url, { cache: "no-store", signal });
  const payload = (await safeJSON(response)) as {
    services?: unknown;
    discovery_stats?: unknown;
    suggestions?: unknown;
    error?: string;
  } | null;
  return { response, payload };
}
