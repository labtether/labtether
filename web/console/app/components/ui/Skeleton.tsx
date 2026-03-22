type SkeletonProps = {
  className?: string;
  width?: string;
  height?: string;
  rounded?: "sm" | "md" | "lg" | "full";
};

export function Skeleton({ className = "", width, height = "12px", rounded = "md" }: SkeletonProps) {
  const radiusClass = rounded === "full" ? "rounded-full" : rounded === "lg" ? "rounded-lg" : rounded === "sm" ? "rounded-sm" : "rounded";
  return (
    <div
      className={`skeleton-shimmer ${radiusClass} ${className}`}
      style={{ width, height }}
    />
  );
}

export function SkeletonRow({ className = "" }: { className?: string }) {
  return (
    <div className={`flex items-center gap-3 py-2 ${className}`}>
      <Skeleton width="8px" height="8px" rounded="full" className="shrink-0" />
      <Skeleton className="flex-1" height="14px" />
      <Skeleton width="60px" height="14px" />
    </div>
  );
}

export function SkeletonCard({ className = "", lines = 3 }: { className?: string; lines?: number }) {
  return (
    <div className={`rounded-lg border border-[var(--line)] p-4 space-y-3 ${className}`}>
      <div className="flex items-center gap-2">
        <Skeleton width="28px" height="28px" rounded="lg" className="shrink-0" />
        <div className="flex-1 space-y-1.5">
          <Skeleton width="60%" height="13px" />
          <Skeleton width="40%" height="10px" />
        </div>
      </div>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} width={`${85 - i * 15}%`} height="11px" />
      ))}
    </div>
  );
}

export function SkeletonMetric({ className = "" }: { className?: string }) {
  return (
    <div className={`space-y-2 ${className}`}>
      <div className="flex items-center justify-between">
        <Skeleton width="80px" height="11px" />
        <Skeleton width="40px" height="18px" rounded="lg" />
      </div>
      <Skeleton width="100%" height="48px" rounded="lg" />
    </div>
  );
}

export function SkeletonTable({ rows = 5, cols = 4, className = "" }: { rows?: number; cols?: number; className?: string }) {
  return (
    <div className={`space-y-0 ${className}`}>
      <div className="flex items-center gap-4 py-2 border-b border-[var(--line)]">
        {Array.from({ length: cols }).map((_, i) => (
          <Skeleton key={i} width={i === 0 ? "30%" : `${20 - i * 2}%`} height="10px" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, row) => (
        <div key={row} className="flex items-center gap-4 py-2.5 border-b border-[var(--line)]/30">
          {Array.from({ length: cols }).map((_, col) => (
            <Skeleton key={col} width={col === 0 ? "30%" : `${20 - col * 2}%`} height="12px" />
          ))}
        </div>
      ))}
    </div>
  );
}
