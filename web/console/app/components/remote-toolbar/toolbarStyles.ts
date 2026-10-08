export const toolbarBg: React.CSSProperties = {
  background:
    "linear-gradient(180deg, rgba(15, 15, 22, 0.82) 0%, rgba(8, 8, 14, 0.90) 100%)",
  backdropFilter: "blur(20px) saturate(1.4)",
  WebkitBackdropFilter: "blur(20px) saturate(1.4)",
  border: "1px solid rgba(255, 255, 255, 0.07)",
};

export const dividerStyle: React.CSSProperties = {
  width: 1,
  alignSelf: "stretch",
  background: "rgba(255, 255, 255, 0.06)",
  margin: "6px 2px",
};

export const moreMenuSectionStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 8,
};

export const moreMenuSectionTitleStyle: React.CSSProperties = {
  fontSize: 10,
  fontWeight: 700,
  letterSpacing: "0.12em",
  textTransform: "uppercase",
  color: "rgba(255,255,255,0.42)",
};

export const moreMenuRowStyle: React.CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: 6,
  alignItems: "center",
};

export const moreMenuShortcutButtonStyle = {
  fontSize: 10,
} satisfies React.CSSProperties;

export const moreMenuActionButtonClass =
  "flex items-center justify-center h-7 rounded-md px-2 font-mono font-semibold text-white/60 hover:text-white/90 hover:bg-white/8 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]";

export function toolbarStyles(
  overlayPosition: "top" | "bottom",
  visible: boolean,
) {
  const moreMenuPanelStyle: React.CSSProperties = {
    ...toolbarBg,
    position: "absolute",
    right: 0,
    minWidth: 248,
    maxWidth: 320,
    zIndex: 80,
    padding: "10px",
    borderRadius: 12,
    border: "1px solid rgba(255,255,255,0.12)",
    boxShadow: "0 16px 40px rgba(0,0,0,0.35)",
    background:
      "linear-gradient(180deg, rgba(15, 15, 22, 0.92) 0%, rgba(8, 8, 14, 0.96) 100%)",
    backdropFilter: "blur(20px) saturate(1.4)",
    top: overlayPosition === "top" ? "calc(100% + 8px)" : undefined,
    bottom: overlayPosition === "bottom" ? "calc(100% + 8px)" : undefined,
  };

  const overlayContainerStyle: React.CSSProperties = {
    position: "absolute",
    left: "50%",
    transform: "translateX(-50%)",
    zIndex: 50,
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    pointerEvents: "none",
    top: overlayPosition === "top" ? 0 : undefined,
    bottom: overlayPosition === "bottom" ? 0 : undefined,
  };

  const overlayBodyStyle: React.CSSProperties = {
    ...toolbarBg,
    borderRadius: overlayPosition === "top" ? "0 0 10px 10px" : "10px 10px 0 0",
    padding: "5px 12px",
    display: "flex",
    alignItems: "center",
    gap: 3,
    opacity: visible ? 1 : 0,
    visibility: visible ? "visible" : "hidden",
    transform: visible
      ? "translateY(0)"
      : overlayPosition === "top"
        ? "translateY(-100%)"
        : "translateY(100%)",
    transition: "opacity 200ms ease, transform 200ms ease",
    pointerEvents: visible ? "auto" : "none",
    boxShadow: "0 4px 16px rgba(0,0,0,0.5)",
  };

  const overlayHandleStyle: React.CSSProperties = {
    ...toolbarBg,
    minWidth: visible ? 72 : 96,
    height: visible ? 18 : 24,
    borderRadius:
      overlayPosition === "top" ? "0 0 999px 999px" : "999px 999px 0 0",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    cursor: "pointer",
    pointerEvents: "auto",
    opacity: visible ? 0.68 : 0.95,
    transition: "opacity 200ms ease, min-width 200ms ease, height 200ms ease",
    boxShadow: visible ? "none" : "0 6px 20px rgba(0,0,0,0.35)",
    border: "none",
    padding: visible ? "0 10px" : "0 12px",
  };
  return {
    moreMenuPanelStyle,
    overlayContainerStyle,
    overlayBodyStyle,
    overlayHandleStyle,
  };
}
