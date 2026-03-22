type MiniBarProps = {
  value: number;
  label?: string;
  className?: string;
};

export function MiniBar({ value, label, className }: MiniBarProps) {
  const clamped = Math.min(100, Math.max(0, value));
  const color =
    clamped > 90 ? "var(--bad)" : clamped > 70 ? "var(--warn)" : "var(--ok)";

  return (
    <span className={`inline-flex items-center gap-1.5 ${className ?? ""}`}>
      <span className="inline-block w-[52px] h-[3px] bg-[var(--surface)] rounded-full overflow-hidden">
        <span
          className="block h-full rounded-full transition-[width] duration-[var(--dur-normal)]"
          style={{
            width: `${clamped}%`,
            background: `linear-gradient(90deg, color-mix(in srgb, ${color} 60%, transparent), ${color})`,
            boxShadow: `0 0 6px color-mix(in srgb, ${color} 30%, transparent)`,
          }}
        />
      </span>
      {label && (
        <span className="text-[10px] font-mono text-[var(--muted)] tabular-nums">
          {label}
        </span>
      )}
    </span>
  );
}
