export default function Loading() {
  return (
    <div className="fixed inset-0 z-20 overflow-hidden md:left-52">
      <div className="flex h-full w-full items-center justify-center bg-[var(--surface)]/60 backdrop-blur-sm">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--muted)]" />
          <p className="text-sm text-[var(--muted)]">Loading topology...</p>
        </div>
      </div>
    </div>
  );
}
