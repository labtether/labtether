"use client";


/* ── helpers ── */

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

export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "Never";
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return "just now";
  const mins = Math.floor(diff / 60_000);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}
