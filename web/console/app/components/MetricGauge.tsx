export function MetricGauge({
  label,
  value,
  unit = "%",
  max = 100,
}: {
  label: string;
  value?: number;
  unit?: string;
  max?: number;
}) {
  const hasValue = typeof value === "number" && Number.isFinite(value);
  const percent = hasValue ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const displayValue = hasValue ? `${value.toFixed(1)}${unit}` : "--";
  const color = !hasValue ? "bg-[var(--surface)]" : percent > 90 ? "bg-[var(--bad)]" : percent > 70 ? "bg-[var(--warn)]" : "bg-[var(--ok)]";
  const textColor = !hasValue ? "text-[var(--muted)]" : percent > 90 ? "text-[var(--bad)]" : percent > 70 ? "text-[var(--warn)]" : "text-[var(--ok)]";

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <span className="text-xs text-[var(--muted)]">{label}</span>
        <span className={`text-xs font-medium tabular-nums ${textColor}`}>{displayValue}</span>
      </div>
      <div className="h-1.5 bg-[var(--surface)] rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full transition-[width] duration-[var(--dur-normal)] ${color}`}
          style={{ width: `${percent}%` }}
        />
      </div>
    </div>
  );
}
