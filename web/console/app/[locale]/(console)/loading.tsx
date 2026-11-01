import { Skeleton, SkeletonTable } from "../../components/ui/Skeleton";

/** 2.0 route loading: a title bar and one sheet of rows, shown only after 300 ms so fast pages never flash it. */
export default function Loading() {
  return (
    <div className="kt-loading" aria-busy="true">
      <Skeleton width="180px" height="20px" className="mb-6" />
      <div className="kt-sheet">
        <SkeletonTable rows={6} cols={4} />
      </div>
    </div>
  );
}
