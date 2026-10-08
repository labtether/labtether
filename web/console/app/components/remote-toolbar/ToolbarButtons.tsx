"use client";

export function SegmentGroup({
  options,
  value,
  onChange,
  label,
}: {
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
  label: string;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex items-center rounded-md overflow-hidden border border-white/10"
    >
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            className={`px-2 py-0.5 font-medium transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)] ${
              active
                ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                : "text-white/50 hover:text-white/80 hover:bg-white/5"
            }`}
            style={{ fontSize: 11 }}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

export function IconButton({
  onClick,
  active,
  disabled,
  danger,
  title,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  danger?: boolean;
  title: string;
  children: React.ReactNode;
}) {
  const base =
    "flex items-center justify-center w-7 h-7 rounded-lg transition-all duration-150 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]";
  const variant = disabled
    ? "text-white/20 cursor-not-allowed"
    : danger
      ? "text-red-400 hover:text-red-300 hover:bg-red-500/15"
      : active
        ? "bg-white/12 text-white/95 shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]"
        : "text-white/45 hover:text-white/85 hover:bg-white/8";
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-label={title}
      disabled={disabled}
      className={`${base} ${variant}`}
    >
      {children}
    </button>
  );
}

export function LabeledIconButton({
  onClick,
  active,
  disabled,
  title,
  label,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  title: string;
  label: string;
  children: React.ReactNode;
}) {
  const base =
    "flex items-center gap-1.5 h-7 rounded-md px-2 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]";
  const variant = disabled
    ? "text-white/25 bg-white/5 cursor-not-allowed"
    : active
      ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
      : "text-white/65 hover:text-white/90 hover:bg-white/8";
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-label={title}
      disabled={disabled}
      className={`${base} ${variant}`}
    >
      <span className="shrink-0">{children}</span>
      <span className="text-[10px] font-semibold uppercase tracking-[0.14em]">
        {label}
      </span>
    </button>
  );
}
