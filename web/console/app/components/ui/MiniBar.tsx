type MiniBarProps = {
  value: number;
  label?: string;
  className?: string;
};

/** Usage at or above this turns amber; below it the bar stays one quiet hue. */
export const HOT_PERCENT = 85;

/** 2.0 meter (kit.css .kt-meter): a 4 px track, a quiet fill, amber from 85%. */
export function MiniBar({ value, label, className }: MiniBarProps) {
  const clamped = Math.min(100, Math.max(0, Number.isFinite(value) ? value : 0));
  const hot = clamped >= HOT_PERCENT;

  return (
    <span className={`kt-minibar${className ? ` ${className}` : ""}`}>
      <span className={`kt-meter${hot ? " hot" : ""}`} aria-hidden="true">
        <i style={{ width: `${clamped}%` }} />
      </span>
      {label && <span className={`kt-minibar-n kt-num${hot ? " kt-hot" : ""}`}>{label}</span>}
    </span>
  );
}
