"use client";

export function ToolbarRevealHandle({
  style,
  resetTimer,
}: {
  style: React.CSSProperties;
  resetTimer: () => void;
}) {
  return (
    <button
      type="button"
      onClick={resetTimer}
      onMouseEnter={resetTimer}
      title="Show remote view tools"
      aria-label="Show remote view tools"
      style={style}
    >
      <div
        style={{
          width: 24,
          height: 3,
          borderRadius: 2,
          backgroundColor: "rgba(255,255,255,0.35)",
        }}
      />
      <span
        style={{
          fontSize: 10,
          fontWeight: 700,
          letterSpacing: "0.08em",
          textTransform: "uppercase",
          color: "rgba(255,255,255,0.82)",
          whiteSpace: "nowrap",
        }}
      >
        Tools
      </span>
    </button>
  );
}
