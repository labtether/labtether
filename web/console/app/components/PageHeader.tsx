import type { ReactNode } from "react";

export function PageHeader({
  title,
  subtitle,
  action,
  className,
}: {
  title: string;
  subtitle?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <header className={`mb-6${className ? ` ${className}` : ""}`}>
      <p className="text-[10px] font-mono font-semibold text-[var(--muted)] mb-1 tracking-[0.06em] uppercase">
        // {title}
      </p>
      <div className="flex items-center justify-between gap-4">
        <h1
          className="text-xl font-semibold font-[family-name:var(--font-heading)] tracking-tight"
          style={{
            background:
              "linear-gradient(135deg, var(--text) 0%, var(--text-secondary) 50%, var(--accent) 150%)",
            WebkitBackgroundClip: "text",
            WebkitTextFillColor: "transparent",
            backgroundClip: "text",
          }}
        >
          {title}
        </h1>
        {action ?? null}
      </div>
      {subtitle ? (
        <p className="text-sm text-[var(--muted)] mt-0.5">{subtitle}</p>
      ) : null}
    </header>
  );
}
