export type CollectorWithBaseURL = {
  collector_type: string;
  config?: Record<string, unknown>;
};

export function stringValue(value: unknown): string {
  if (typeof value === "string") {
    return value.trim();
  }
  if (typeof value === "number") {
    return String(value);
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  return "";
}

export function boolValue(value: unknown, fallback: boolean): boolean {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "string") {
    const lowered = value.trim().toLowerCase();
    if (lowered === "true") return true;
    if (lowered === "false") return false;
  }
  if (typeof value === "number") {
    return value !== 0;
  }
  return fallback;
}

export function numberValue(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return fallback;
}

export function clampInterval(value: number): number {
  if (!Number.isFinite(value) || value <= 0) {
    return 60;
  }
  if (value < 15) return 15;
  if (value > 3600) return 3600;
  return Math.floor(value);
}

export function normalizeBaseURL(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  try {
    const parsed = new URL(trimmed);
    const protocol = parsed.protocol.toLowerCase();
    const hostname = parsed.hostname.toLowerCase();
    const port = parsed.port ? `:${parsed.port}` : "";
    const pathname = parsed.pathname.replace(/\/+$/g, "");
    return `${protocol}//${hostname}${port}${pathname}`;
  } catch {
    return trimmed.replace(/\/+$/g, "");
  }
}

export function findCollectorByBaseURL<T extends CollectorWithBaseURL>(
  collectors: T[],
  collectorType: string,
  normalizedBaseURL: string,
): T | undefined {
  const typedCollectors = collectors.filter((entry) => entry.collector_type === collectorType);
  const exact = typedCollectors.find((entry) => (
    normalizeBaseURL(stringValue(entry.config?.["base_url"])) === normalizedBaseURL
  ));
  if (exact) return exact;

  if (typedCollectors.length === 1) {
    return typedCollectors[0];
  }
  return undefined;
}

export function hostLabelFromURL(value: string, fallback: string): string {
  try {
    const parsed = new URL(value);
    return parsed.hostname || fallback;
  } catch {
    return fallback;
  }
}

export function slugify(value: string, fallback: string): string {
  const trimmed = value.trim().toLowerCase();
  if (!trimmed) return fallback;
  const slug = trimmed
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+/g, "")
    .replace(/-+$/g, "");
  return slug || fallback;
}

export async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
