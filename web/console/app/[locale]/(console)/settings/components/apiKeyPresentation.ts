/** The ISO time a new key expires for an expiry choice (30d, 90d, 1y); null for never. */
export function expiryToIso(value: string): string | null {
  if (value === "never") return null;
  const now = Date.now();
  const msPerDay = 86_400_000;
  switch (value) {
    case "30d":
      return new Date(now + 30 * msPerDay).toISOString();
    case "90d":
      return new Date(now + 90 * msPerDay).toISOString();
    case "1y":
      return new Date(now + 365 * msPerDay).toISOString();
    default:
      return null;
  }
}
