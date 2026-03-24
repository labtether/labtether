import { SkeletonRow, SkeletonTable } from "../../../components/ui/Skeleton";

export default function Loading() {
  return (
    <div className="animate-fade-in p-4 space-y-4">
      <SkeletonRow />
      <SkeletonTable rows={6} cols={4} />
    </div>
  );
}
