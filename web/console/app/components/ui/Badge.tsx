import { getStatusLabel, statusConfig } from "../../lib/status";

type BadgeProps = {
  status: string;
  size?: "sm" | "md";
  dot?: boolean;
  className?: string;
};

type Tone = "ok" | "warn" | "bad" | "off" | "busy";

/** Work in progress is not a problem: a neutral dot that pulses, not amber. */
const busyStatuses = new Set(["pending", "running", "in_progress"]);

const toneByColor: Record<string, Tone> = { emerald: "ok", amber: "warn", red: "bad", zinc: "off" };

function toneOf(status: string): Tone | null {
  const key = status.toLowerCase();
  if (busyStatuses.has(key)) return "busy";
  const entry = statusConfig[key];
  return entry ? toneByColor[entry.color] ?? "off" : null;
}

/**
 * 2.0 status: a dot and a word (kit.css .kt-st). The word takes the tone colour only for a problem.
 * A label that is not a known status (a key type, a version) is a quiet tag with no dot.
 */
export function Badge({ status, size = "md", dot = false, className = "" }: BadgeProps) {
  const label = getStatusLabel(status);
  const tone = toneOf(status);
  const small = size === "sm" ? " sm" : "";
  const extra = className ? ` ${className}` : "";

  if (dot) {
    return (
      <span
        className={`kt-dot${small} ${tone ?? "off"}${extra}`}
        role="img"
        aria-label={label}
        title={label}
      />
    );
  }

  if (!tone) {
    return <span className={`kt-tag${extra}`}>{label}</span>;
  }

  const wordTone = tone === "bad" || tone === "warn" ? ` ${tone}` : "";
  return (
    <span className={`kt-st${small}${wordTone}${extra}`}>
      <span className={`kt-dot${small} ${tone}`} aria-hidden="true" />
      {label}
    </span>
  );
}
