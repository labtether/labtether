/** Ranges a device chart can show. Logs keep the shorter `telemetryWindows`. */
export const metricWindows = ["15m", "1h", "6h", "24h", "7d", "30d"] as const;
export type MetricWindow = (typeof metricWindows)[number];

const longWindowParams: Partial<Record<MetricWindow, string>> = { "7d": "168h", "30d": "720h" };

/**
 * The `window` value to send for a chart range. The Hub reads it as a Go duration, which has no day unit: "7d"
 * does not parse and the Hub answers with one hour, so days go as hours.
 */
export function metricWindowParam(window: MetricWindow): string {
  return longWindowParams[window] ?? window;
}
