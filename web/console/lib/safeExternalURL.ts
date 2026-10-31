/**
 * Returns the URL when it is an absolute http(s) link, otherwise undefined.
 * Service URLs can come from agents, so a compromised host could report a
 * javascript: or data: URL. Links and window.open must only use this result.
 */
export function safeHttpURL(raw: string | null | undefined): string | undefined {
  const value = raw?.trim();
  if (!value) return undefined;
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:" ? parsed.href : undefined;
  } catch {
    return undefined;
  }
}
