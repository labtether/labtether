import { getStatusColors, getStatusLabel, statusConfig } from "../../lib/status";

type BadgeProps = {
  status: string;
  size?: "sm" | "md";
  dot?: boolean;
  className?: string;
};

/** Map status color category to the CSS custom property for glow. */
function getGlowStyle(status: string): React.CSSProperties {
  const entry = statusConfig[status.toLowerCase()];
  const color = entry?.color ?? "zinc";

  switch (color) {
    case "emerald":
      return { boxShadow: "0 0 4px var(--ok-glow), 0 0 12px var(--ok-glow)" };
    case "amber":
      return { boxShadow: "0 0 4px var(--warn-glow), 0 0 12px var(--warn-glow)" };
    case "red":
      return { boxShadow: "0 0 4px var(--bad-glow), 0 0 12px var(--bad-glow)" };
    default:
      return {};
  }
}

/** Critical-severity statuses that get a pulse animation on the dot. */
const criticalStatuses = new Set(["critical", "firing", "offline", "bad", "failed"]);
/** Healthy statuses that get a gentle pulse to indicate liveliness. */
const aliveStatuses = new Set(["online", "ok", "healthy", "running", "active"]);

function isCritical(status: string): boolean {
  return criticalStatuses.has(status.toLowerCase());
}

function isAlive(status: string): boolean {
  return aliveStatuses.has(status.toLowerCase());
}

export function Badge({ status, size = "md", dot = false, className = "" }: BadgeProps) {
  const colors = getStatusColors(status);
  const label = getStatusLabel(status);
  const dotSize = size === "sm" ? "h-1.5 w-1.5" : "h-2 w-2";
  const textSize = size === "sm" ? "text-xs" : "text-sm";

  const glowStyle = getGlowStyle(status);
  const pulseStyle: React.CSSProperties = isCritical(status)
    ? { ...glowStyle, animation: "status-glow 1.5s ease-in-out infinite" }
    : isAlive(status)
      ? { ...glowStyle, animation: "pulse-dot 2.5s ease-in-out infinite" }
      : glowStyle;

  if (dot) {
    return (
      <span
        className={`inline-block rounded-full ${dotSize} ${colors.dot} ${className}`}
        style={pulseStyle}
        title={label}
      />
    );
  }

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-0.5 ${textSize} font-medium ${colors.bg} ${colors.text} ${className}`}
    >
      <span
        className={`inline-block rounded-full ${dotSize} ${colors.dot}`}
        style={pulseStyle}
      />
      {label}
    </span>
  );
}
