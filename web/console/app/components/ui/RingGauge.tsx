type RingGaugeProps = {
  value?: number;
  max?: number;
  label: string;
  unit?: string;
  size?: number;
};

export function RingGauge({ value, max = 100, label, unit = "%", size = 72 }: RingGaugeProps) {
  const hasValue = typeof value === "number" && Number.isFinite(value);
  const percent = hasValue ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const displayValue = hasValue ? `${Math.round(value)}` : "--";

  // SVG ring math
  const strokeWidth = 5;
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (percent / 100) * circumference;

  // Color based on percentage thresholds
  const strokeColor = !hasValue
    ? "var(--muted)"
    : percent > 90
      ? "var(--bad)"
      : percent > 70
        ? "var(--warn)"
        : "var(--ok)";

  const textColor = !hasValue
    ? "text-[var(--muted)]"
    : percent > 90
      ? "text-[var(--bad)]"
      : percent > 70
        ? "text-[var(--warn)]"
        : "text-[var(--ok)]";

  return (
    <div className="flex flex-col items-center gap-1">
      <div className="relative" style={{ width: size, height: size }}>
        <svg width={size} height={size} className="-rotate-90" role="img" aria-label={hasValue ? `${label}: ${displayValue}${unit}` : `${label}: no data`}>
          {/* Background track */}
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke="var(--surface)"
            strokeWidth={strokeWidth}
          />
          {/* Value arc */}
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke={strokeColor}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={hasValue ? offset : circumference}
            style={{
              transition: "stroke-dashoffset 0.6s ease, stroke 0.3s ease",
              ...(hasValue ? {} : { strokeDasharray: "4 8" }),
            }}
          />
        </svg>
        {/* Center value */}
        <div className="absolute inset-0 flex items-center justify-center">
          <span className={`text-sm font-semibold tabular-nums ${textColor}`}>
            {displayValue}
            {hasValue && <span className="text-[10px] font-normal">{unit}</span>}
          </span>
        </div>
      </div>
      <span className="text-[10px] text-[var(--muted)] uppercase tracking-wider">{label}</span>
    </div>
  );
}
