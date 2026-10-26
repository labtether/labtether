// TrueNAS returns ZFS properties as {parsed, rawvalue, value} objects.
// Decode only documented scalar forms; do not stringify objects into UI text.
export type Row = Record<string, unknown>;

export function record(value: unknown): Row | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as Row : undefined;
}

export function requireRecord(value: unknown, label: string): Row {
  const row = record(value);
  if (!row) throw new Error(`invalid ${label} response: expected object`);
  return row;
}

export function property(value: unknown): unknown {
  const row = record(value);
  if (!row) return value;
  for (const key of ["parsed", "rawvalue", "value"]) {
    if (row[key] !== null && row[key] !== undefined) return row[key];
  }
  return undefined;
}

export function text(value: unknown): string | undefined {
  const scalar = property(value);
  return typeof scalar === "string" ? scalar : undefined;
}

export function requireText(value: unknown, label: string): string {
  const result = text(value);
  if (!result?.trim()) throw new Error(`invalid ${label} response: missing text`);
  return result;
}

export function number(value: unknown): number | undefined {
  const scalar = property(value);
  if (typeof scalar === "number") return Number.isFinite(scalar) ? scalar : undefined;
  if (typeof scalar !== "string" || !/^-?\d+(?:\.\d+)?$/.test(scalar.trim())) return undefined;
  const parsed = Number(scalar);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function bool(value: unknown): boolean | undefined {
  const scalar = property(value);
  if (typeof scalar === "boolean") return scalar;
  if (typeof scalar !== "string") return undefined;
  switch (scalar.trim().toLowerCase()) {
    case "on": case "yes": case "true": return true;
    case "off": case "no": case "false": return false;
    default: return undefined;
  }
}

export function requireBool(value: unknown, label: string): boolean {
  const result = bool(value);
  if (result === undefined) throw new Error(`invalid ${label} response: missing boolean`);
  return result;
}

export function identifier(value: unknown, label: string): string | number {
  if (typeof value === "string" && value.trim()) return value;
  if (typeof value === "number" && Number.isSafeInteger(value)) return value;
  throw new Error(`invalid ${label} response: missing id`);
}

export function strings(value: unknown): string[] | undefined {
  return Array.isArray(value) && value.every((item) => typeof item === "string")
    ? value : undefined;
}

// JSON-RPC dates are ISO strings or the legacy {$date: epochMilliseconds} form.
export function timestamp(value: unknown): string | undefined {
  const legacy = record(value)?.$date;
  const raw = typeof legacy === "number" ? legacy : typeof value === "string" ? value : undefined;
  if (raw === undefined) return undefined;
  const date = new Date(raw);
  return Number.isFinite(date.getTime()) ? date.toISOString() : undefined;
}
