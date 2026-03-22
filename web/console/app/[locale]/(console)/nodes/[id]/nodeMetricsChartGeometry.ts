export type ChartPoint = {
  x: number;
  y: number;
  ts: number;
  value: number;
};

export const CHART_TOP = 8;
export const CHART_BOTTOM = 92;

export function valueToChartY(value: number, yMin: number, yMax: number): number {
  const normalized = (value - yMin) / Math.max(yMax - yMin, 1e-9);
  const unclamped = CHART_BOTTOM - normalized * (CHART_BOTTOM - CHART_TOP);
  return clamp(unclamped, CHART_TOP, CHART_BOTTOM);
}

export function buildLinePath(points: ChartPoint[]): string {
  if (points.length === 0) return "";
  if (points.length === 1) {
    const point = points[0];
    return `M ${point.x.toFixed(2)} ${point.y.toFixed(2)}`;
  }
  return points
    .map((point, idx) => `${idx === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`)
    .join(" ");
}

export function buildAreaPath(points: ChartPoint[]): string {
  if (points.length === 0) return "";
  const line = buildLinePath(points);
  const first = points[0];
  const last = points[points.length - 1];
  return `${line} L ${last.x.toFixed(2)} ${CHART_BOTTOM} L ${first.x.toFixed(2)} ${CHART_BOTTOM} Z`;
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
