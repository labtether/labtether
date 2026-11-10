/**
 * The viewer's own time zone. next-intl formats in the zone the server rendered with, and the Hub's container runs
 * in UTC, so a time shown through `useFormatter()` must name the viewer's zone or it reads hours off.
 */
export function viewerTimeZone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
  } catch {
    return undefined;
  }
}
