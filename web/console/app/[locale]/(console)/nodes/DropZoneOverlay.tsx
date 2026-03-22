'use client';

type DropZoneOverlayProps = {
  /** Whether a compatible drag is hovering over this card. */
  isActive: boolean;
  /** Which half is being hovered. */
  dropHalf: 'top' | 'bottom' | null;
  /** Whether the bottom (nest) zone is available for this target asset. */
  canNest?: boolean;
  onDragOver: (e: React.DragEvent) => void;
  onDragLeave: (e: React.DragEvent) => void;
  onDrop: (e: React.DragEvent) => void;
};

/**
 * DropZoneOverlay renders split drop zones over a device row.
 *
 * The parent element must have `position: relative` for this overlay to
 * position correctly.
 *
 * - Top half: "Merge as same device" — indigo highlight
 * - Bottom half: "Nest as child" — purple highlight (only when canNest is true)
 */
export function DropZoneOverlay({
  isActive,
  dropHalf,
  canNest = false,
  onDragOver,
  onDragLeave,
  onDrop,
}: DropZoneOverlayProps) {
  if (!isActive) return null;

  return (
    <div
      className="absolute inset-0 z-10 flex flex-col pointer-events-none"
      aria-hidden="true"
    >
      {/* Top half — merge */}
      <div
        className={`flex-1 flex items-center justify-center text-[10px] font-semibold rounded-t-md transition-colors pointer-events-auto select-none
          ${dropHalf === 'top'
            ? 'bg-indigo-500/25 text-indigo-300 ring-1 ring-inset ring-indigo-500/60'
            : 'bg-indigo-500/10 text-indigo-400/70'
          }`}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
      >
        <span className={`px-2 py-0.5 rounded ${dropHalf === 'top' ? 'opacity-100' : 'opacity-50'}`}>
          Merge as same device
        </span>
      </div>

      {/* Bottom half — nest */}
      {canNest ? (
        <div
          className={`flex-1 flex items-center justify-center text-[10px] font-semibold rounded-b-md transition-colors pointer-events-auto select-none
            ${dropHalf === 'bottom'
              ? 'bg-purple-500/25 text-purple-300 ring-1 ring-inset ring-purple-500/60'
              : 'bg-purple-500/10 text-purple-400/70'
            }`}
          onDragOver={onDragOver}
          onDragLeave={onDragLeave}
          onDrop={onDrop}
        >
          <span className={`px-2 py-0.5 rounded ${dropHalf === 'bottom' ? 'opacity-100' : 'opacity-50'}`}>
            Nest as child
          </span>
        </div>
      ) : null}
    </div>
  );
}
