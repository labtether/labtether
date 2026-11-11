// The Proxmox Metrics view's data: the Hub's RRD answer for one resource.

export type MetricPoint = {
  time?: number;
  cpu?: number;
  mem?: number;
  /** Proxmox's RRD name for memory in use; the Hub passes it through. */
  memused?: number;
  maxmem?: number;
  disk?: number;
  maxdisk?: number;
  netin?: number;
  netout?: number;
  diskread?: number;
  diskwrite?: number;
};

export type MetricsResponse = {
  /** The Hub answers with points; data is kept for an older Hub. */
  points?: MetricPoint[];
  data?: MetricPoint[];
  timeframe?: string;
};

/** The samples in a metrics answer. The tab read only `data`, so it always said "No samples". */
export function metricPoints(response: MetricsResponse | MetricPoint[] | null | undefined): MetricPoint[] {
  if (!response) return [];
  if (Array.isArray(response)) return response;
  return response.points ?? response.data ?? [];
}
