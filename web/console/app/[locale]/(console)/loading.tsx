import { SkeletonCard, SkeletonRow } from "../../components/ui/Skeleton";

export default function Loading() {
  return (
    <div className="animate-fade-in p-4 space-y-4">
      {/* KPI row skeleton */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
        {Array.from({ length: 4 }).map((_, i) => (
          <SkeletonCard key={i} lines={1} />
        ))}
      </div>
      {/* Content skeleton */}
      <SkeletonCard lines={4} />
      <div className="space-y-1">
        {Array.from({ length: 5 }).map((_, i) => (
          <SkeletonRow key={i} />
        ))}
      </div>
    </div>
  );
}
