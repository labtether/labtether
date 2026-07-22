function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

export function persistentSessionIDFromPayload(payload: unknown): string {
  const root = asRecord(payload);
  if (!root) return "";

  for (const field of ["persistent_session", "session"] as const) {
    const nested = asRecord(root[field]);
    if (typeof nested?.id === "string" && nested.id.trim()) {
      return nested.id.trim();
    }
  }
  return typeof root.id === "string" ? root.id.trim() : "";
}
