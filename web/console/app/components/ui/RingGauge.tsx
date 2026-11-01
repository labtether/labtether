import { HOT_PERCENT } from "./MiniBar";

type RingGaugeProps = {
  value?: number;
  max?: number;
  label: string;
  unit?: string;
  size?: number;
};

/** 2.0 ring gauge: one quiet hue on a track, amber from 85% (the meter rule). */
export function RingGauge({ value, max = 100, label, unit = "%", size = 72 }: RingGaugeProps) {
  const hasValue = typeof value === "number" && Number.isFinite(value);
  const percent = hasValue ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const displayValue = hasValue ? `${Math.round(value)}` : "—";
  const hot = hasValue && percent >= HOT_PERCENT;

  const strokeWidth = 4;
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (percent / 100) * circumference;

  return (
    <div className="kt-ring">
      <div className="kt-ring-c" style={{ width: size, height: size }}>
        <svg
          width={size}
          height={size}
          role="img"
          aria-label={hasValue ? `${label}: ${displayValue}${unit}` : `${label}: no data`}
        >
          <circle className="kt-ring-t" cx={size / 2} cy={size / 2} r={radius} strokeWidth={strokeWidth} />
          {hasValue ? (
            <circle
              className={`kt-ring-a${hot ? " hot" : ""}`}
              cx={size / 2}
              cy={size / 2}
              r={radius}
              strokeWidth={strokeWidth}
              strokeDasharray={circumference}
              strokeDashoffset={offset}
            />
          ) : null}
        </svg>
        <span className={`kt-ring-v kt-num${hot ? " kt-hot" : ""}`} aria-hidden="true">
          <span>
            {displayValue}
            {hasValue ? <small>{unit}</small> : null}
          </span>
        </span>
      </div>
      <span className="kt-ring-l">{label}</span>
    </div>
  );
}
