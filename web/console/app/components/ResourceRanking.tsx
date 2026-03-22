import { Link } from "../../i18n/navigation";

type ResourceRankingProps = {
  items: Array<{ id: string; name: string; value: number }>;
  className?: string;
};

export function ResourceRanking({ items, className }: ResourceRankingProps) {
  if (items.length === 0) {
    return (
      <div className={className}>
        <p className="text-xs text-[var(--muted)] py-3 text-center">No data</p>
      </div>
    );
  }

  return (
    <div className={className}>
      <ul className="space-y-2">
        {items.map((item, index) => {
          const clamped = Math.min(100, Math.max(0, item.value));
          const isWarn = clamped >= 80;

          return (
            <li key={item.id} className="flex items-center gap-2.5">
              {/* Rank */}
              <span className="w-5 text-right text-xs font-mono text-[var(--muted)] shrink-0 tabular-nums">
                {index + 1}
              </span>

              {/* Device name */}
              <Link
                href={`/nodes/${item.id}`}
                className="flex-1 min-w-0 text-xs font-mono text-[var(--text)] truncate hover:text-[var(--accent-text)] transition-colors"
                style={{ transitionDuration: "var(--dur-fast)" }}
              >
                {item.name}
              </Link>

              {/* Bar */}
              <div className="w-14 h-[3px] rounded-full bg-[var(--surface)] overflow-hidden shrink-0">
                <div
                  className="h-full rounded-full transition-[width]"
                  style={{
                    width: `${clamped}%`,
                    background: isWarn
                      ? "linear-gradient(90deg, var(--color-warn), var(--color-bad))"
                      : "linear-gradient(90deg, var(--accent), var(--accent-text))",
                    boxShadow: isWarn
                      ? "0 0 6px var(--warn-glow)"
                      : "0 0 6px var(--accent-glow)",
                    transitionDuration: "var(--dur-slow)",
                    transitionTimingFunction: "var(--ease-out)",
                  }}
                />
              </div>

              {/* Value */}
              <span
                className={`w-10 text-right text-xs font-mono tabular-nums shrink-0 ${
                  isWarn ? "text-[var(--color-warn)]" : "text-[var(--text)]"
                }`}
              >
                {clamped.toFixed(0)}%
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
