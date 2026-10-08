// Narrow untrusted service API fields before they enter console state.

export function asObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

export function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function asFiniteNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

export function asBoolean(value: unknown): boolean {
  return value === true;
}

export function asRecordNumber(value: unknown): Record<string, number> | undefined {
  const raw = asObject(value);
  if (!raw) {
    return undefined;
  }
  const normalized: Record<string, number> = {};
  for (const [key, entry] of Object.entries(raw)) {
    if (typeof entry === "number" && Number.isFinite(entry)) {
      normalized[key] = entry;
    }
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined;
}
