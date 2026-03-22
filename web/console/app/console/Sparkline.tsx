export function Sparkline({ points }: { points: Array<{ value: number }> }) {
  if (points.length < 2) {
    return <div className="sparklineEmpty">no series</div>;
  }

  const sampled = points.length > 80 ? points.filter((_, i) => i % Math.ceil(points.length / 80) === 0) : points;
  const values = sampled.map((point) => point.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;

  const coords = sampled
    .map((point, index) => {
      const x = (index / (sampled.length - 1)) * 100;
      const y = 30 - ((point.value - min) / span) * 28;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");

  return (
    <svg className="sparkline" viewBox="0 0 100 32" preserveAspectRatio="none" aria-hidden>
      <polyline points={coords} className="sparklineLine" />
    </svg>
  );
}
