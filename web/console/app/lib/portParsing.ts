const DECIMAL_PORT_PATTERN = /^[0-9]+$/;

export function parsePortInput(raw: unknown, fallback: number): number {
  const fallbackPort = normalizeFallback(fallback);
  const value = String(raw ?? "").trim();
  if (!DECIMAL_PORT_PATTERN.test(value)) {
    return fallbackPort;
  }

  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < 1 || parsed > 65535) {
    return fallbackPort;
  }
  return parsed;
}

function normalizeFallback(fallback: number): number {
  return Number.isSafeInteger(fallback) && fallback >= 1 && fallback <= 65535
    ? fallback
    : 22;
}
