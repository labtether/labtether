type SkeletonProps = {
  className?: string;
  width?: string;
  height?: string;
  rounded?: "sm" | "md" | "lg" | "full";
};

/** 2.0 skeleton (kit.css .kt-skel): a flat bar in the selection fill, no shimmer. */
export function Skeleton({ className = "", width, height = "12px", rounded = "md" }: SkeletonProps) {
  return (
    <div
      className={`kt-skel${rounded === "full" ? " round" : ""}${className ? ` ${className}` : ""}`}
      style={{ width, height }}
      aria-hidden="true"
    />
  );
}

export function SkeletonRow({ className = "" }: { className?: string }) {
  return (
    <div className={`flex items-center gap-3 py-2.5 ${className}`}>
      <Skeleton className="flex-1" height="10px" />
      <Skeleton width="56px" height="10px" />
    </div>
  );
}

export function SkeletonCard({ className = "", lines = 3 }: { className?: string; lines?: number }) {
  return (
    <div className={`kt-sheet space-y-3 ${className}`}>
      <Skeleton width="40%" height="12px" />
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} width={`${85 - i * 15}%`} height="10px" />
      ))}
    </div>
  );
}

export function SkeletonMetric({ className = "" }: { className?: string }) {
  return (
    <div className={`space-y-2 ${className}`}>
      <div className="flex items-center justify-between">
        <Skeleton width="80px" height="10px" />
        <Skeleton width="40px" height="16px" />
      </div>
      <Skeleton width="100%" height="48px" />
    </div>
  );
}

export function SkeletonTable({ rows = 5, cols = 4, className = "" }: { rows?: number; cols?: number; className?: string }) {
  return (
    <div className={`kt-skel-rows ${className}`}>
      {Array.from({ length: rows + 1 }).map((_, row) => (
        <div key={row} className="flex items-center gap-4 py-2.5">
          {Array.from({ length: cols }).map((_, col) => (
            <Skeleton key={col} width={col === 0 ? "30%" : `${20 - col * 2}%`} height={row === 0 ? "8px" : "10px"} />
          ))}
        </div>
      ))}
    </div>
  );
}
