const FALLBACK_TIMEZONES: readonly string[] = [
  "UTC",
  "Etc/UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Phoenix",
  "America/Anchorage",
  "Pacific/Honolulu",
  "America/Toronto",
  "America/Vancouver",
  "America/Mexico_City",
  "America/Sao_Paulo",
  "America/Bogota",
  "America/Lima",
  "America/Caracas",
  "America/Argentina/Buenos_Aires",
  "Europe/London",
  "Europe/Dublin",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Madrid",
  "Europe/Rome",
  "Europe/Amsterdam",
  "Europe/Brussels",
  "Europe/Zurich",
  "Europe/Stockholm",
  "Europe/Warsaw",
  "Europe/Prague",
  "Europe/Vienna",
  "Europe/Copenhagen",
  "Europe/Oslo",
  "Europe/Helsinki",
  "Europe/Athens",
  "Europe/Bucharest",
  "Europe/Istanbul",
  "Europe/Kyiv",
  "Europe/Moscow",
  "Africa/Cairo",
  "Africa/Johannesburg",
  "Africa/Nairobi",
  "Africa/Lagos",
  "Asia/Jerusalem",
  "Asia/Dubai",
  "Asia/Riyadh",
  "Asia/Tehran",
  "Asia/Karachi",
  "Asia/Kolkata",
  "Asia/Dhaka",
  "Asia/Bangkok",
  "Asia/Singapore",
  "Asia/Kuala_Lumpur",
  "Asia/Jakarta",
  "Asia/Manila",
  "Asia/Hong_Kong",
  "Asia/Taipei",
  "Asia/Shanghai",
  "Asia/Seoul",
  "Asia/Tokyo",
  "Asia/Vladivostok",
  "Australia/Perth",
  "Australia/Adelaide",
  "Australia/Sydney",
  "Australia/Brisbane",
  "Australia/Hobart",
  "Pacific/Auckland",
  "Pacific/Fiji",
];

let cachedTimezones: string[] | null = null;

export function listEligibleTimezones(): string[] {
  if (cachedTimezones) return cachedTimezones;

  const supportedValuesOf = (Intl as unknown as {
    supportedValuesOf?: (key: string) => string[];
  }).supportedValuesOf;

  if (typeof supportedValuesOf === "function") {
    try {
      const fromRuntime = supportedValuesOf("timeZone");
      if (fromRuntime.length > 0) {
        cachedTimezones = normalizeTimezones(fromRuntime);
        return cachedTimezones;
      }
    } catch {
      // Fall back to a portable list if runtime support is unavailable.
    }
  }

  cachedTimezones = normalizeTimezones(FALLBACK_TIMEZONES);
  return cachedTimezones;
}

function normalizeTimezones(input: readonly string[]): string[] {
  const unique = new Set<string>();
  for (const timezone of input) {
    const value = timezone.trim();
    if (value) unique.add(value);
  }
  unique.add("UTC");
  return [...unique].sort((left, right) => left.localeCompare(right));
}
