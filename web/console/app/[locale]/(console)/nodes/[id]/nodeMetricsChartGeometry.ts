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

/**
 * A smooth line through the points that never overshoots them (monotone cubic, Fritsch–Carlson), so a flat run
 * stays flat and a peak is drawn at its real height.
 */
export function buildSmoothPath(points: ChartPoint[]): string {
  if (points.length < 3) return buildLinePath(points);
  const n = points.length;
  const dx: number[] = [];
  const slope: number[] = [];
  for (let i = 0; i < n - 1; i += 1) {
    dx.push(points[i + 1].x - points[i].x);
    slope.push(dx[i] === 0 ? 0 : (points[i + 1].y - points[i].y) / dx[i]);
  }
  const tangent: number[] = [slope[0]];
  for (let i = 1; i < n - 1; i += 1) {
    tangent.push(slope[i - 1] * slope[i] <= 0 ? 0 : (slope[i - 1] + slope[i]) / 2);
  }
  tangent.push(slope[n - 2]);
  for (let i = 0; i < n - 1; i += 1) {
    if (slope[i] === 0) {
      tangent[i] = 0;
      tangent[i + 1] = 0;
      continue;
    }
    const a = tangent[i] / slope[i];
    const b = tangent[i + 1] / slope[i];
    const h = a * a + b * b;
    if (h > 9) {
      const t = 3 / Math.sqrt(h);
      tangent[i] = t * a * slope[i];
      tangent[i + 1] = t * b * slope[i];
    }
  }
  const f = (v: number) => v.toFixed(2);
  let d = `M ${f(points[0].x)} ${f(points[0].y)}`;
  for (let i = 0; i < n - 1; i += 1) {
    const p = points[i];
    const q = points[i + 1];
    const third = dx[i] / 3;
    d += ` C ${f(p.x + third)} ${f(p.y + tangent[i] * third)} ${f(q.x - third)} ${f(q.y - tangent[i + 1] * third)} ${f(q.x)} ${f(q.y)}`;
  }
  return d;
}

/** The smooth line closed down to the chart's baseline, for the flat fill under it. */
export function buildSmoothAreaPath(points: ChartPoint[]): string {
  if (points.length === 0) return "";
  const first = points[0];
  const last = points[points.length - 1];
  return `${buildSmoothPath(points)} L ${last.x.toFixed(2)} ${CHART_BOTTOM} L ${first.x.toFixed(2)} ${CHART_BOTTOM} Z`;
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
