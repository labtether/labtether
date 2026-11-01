import { HOT_PERCENT } from "./ui/MiniBar";

/** 2.0 labelled meter: the label and value on one line, a 4 px bar under them; amber from 85%. */
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
  const displayValue = hasValue ? `${value.toFixed(1)}${unit}` : "—";
  const hot = hasValue && percent >= HOT_PERCENT;

  return (
    <div>
      <div className="kt-gauge-h">
        <span className="kt-gauge-l">{label}</span>
        <span className={`kt-gauge-v kt-num${hot ? " kt-hot" : ""}`}>{displayValue}</span>
      </div>
      <div className={`kt-meter${hot ? " hot" : ""}`} aria-hidden="true">
        <i style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}
