"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronsDown, ChevronsUp, Clock3 } from "lucide-react";
import type { RemoteViewToolbarProps } from "./remote-toolbar/types";
import { IconButton } from "./remote-toolbar/ToolbarButtons";
import {
  ToolbarStatus,
  ToolbarStatusBadges,
  ToolbarQuality,
  ToolbarDisplayPicker,
  ToolbarScaling,
} from "./remote-toolbar/ToolbarDisplayControls";
import {
  ToolbarPointerLock,
  ToolbarViewOnly,
} from "./remote-toolbar/ToolbarInputControls";
import {
  ToolbarRecording,
  ToolbarScreenshot,
  ToolbarAudio,
  ToolbarFullscreen,
  ToolbarDisconnect,
} from "./remote-toolbar/ToolbarMediaControls";
import { useToolbarTransferState } from "./remote-toolbar/ToolbarTransferControls";
import { ToolbarMoreMenu } from "./remote-toolbar/ToolbarMoreMenu";
import { ToolbarRevealHandle } from "./remote-toolbar/ToolbarRevealHandle";
import { dividerStyle, toolbarStyles } from "./remote-toolbar/toolbarStyles";

export type {
  RemoteViewToolbarProps,
  RemoteViewToolbarLayout,
  ScalingMode,
} from "./remote-toolbar/types";

const AUTOHIDE_DELAY = 5000;
const HOT_ZONE_PX = 64;

export function RemoteViewToolbar(props: RemoteViewToolbarProps) {
  const { layout = "overlay", connectionState } = props;
  const transfer = useToolbarTransferState(props.clipboardLastSync);
  const isOverlayLayout = layout === "overlay";
  const [visible, setVisible] = useState(true);
  const [overlayPosition, setOverlayPosition] = useState<"top" | "bottom">(
    "bottom",
  );
  const [autoHideEnabled, setAutoHideEnabled] = useState(false);

  const [showMoreMenu, setShowMoreMenu] = useState(false);

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const toolbarRef = useRef<HTMLDivElement>(null);
  const moreMenuRef = useRef<HTMLDivElement>(null);
  const visibleRef = useRef(visible);
  visibleRef.current = visible;

  // Auto-hide timer management
  const resetTimer = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    setVisible(true);
    if (isOverlayLayout && autoHideEnabled) {
      timerRef.current = setTimeout(() => setVisible(false), AUTOHIDE_DELAY);
    }
  }, [autoHideEnabled, isOverlayLayout]);

  // Start timer on mount / when overlay behavior changes
  useEffect(() => {
    if (!isOverlayLayout) {
      setVisible(true);
      return;
    }
    if (!autoHideEnabled) {
      if (timerRef.current) clearTimeout(timerRef.current);
      setVisible(true);
      return;
    }
    resetTimer();
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [autoHideEnabled, isOverlayLayout, resetTimer]);

  // Track mouse movement in container (parent)
  useEffect(() => {
    if (!isOverlayLayout) return;
    const container = toolbarRef.current?.parentElement;
    if (!container) return;

    const handleMouseMove = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect();
      const relativeY = e.clientY - rect.top;
      const inRevealZone =
        overlayPosition === "top"
          ? relativeY <= HOT_ZONE_PX
          : relativeY >= rect.height - HOT_ZONE_PX;

      if (!visibleRef.current && autoHideEnabled && inRevealZone) {
        resetTimer();
      } else if (visibleRef.current && autoHideEnabled) {
        resetTimer();
      }
    };

    container.addEventListener("mousemove", handleMouseMove);
    return () => container.removeEventListener("mousemove", handleMouseMove);
  }, [autoHideEnabled, isOverlayLayout, overlayPosition, resetTimer]);

  useEffect(() => {
    if (!showMoreMenu) return;

    const handlePointerDown = (event: MouseEvent) => {
      if (!moreMenuRef.current?.contains(event.target as Node)) {
        setShowMoreMenu(false);
      }
    };

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setShowMoreMenu(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleEscape);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleEscape);
    };
  }, [showMoreMenu]);

  if (connectionState !== "connected") return null;

  const {
    moreMenuPanelStyle,
    overlayContainerStyle,
    overlayBodyStyle,
    overlayHandleStyle,
  } = toolbarStyles(overlayPosition, visible);
  const displayAvailable =
    props.displays && props.displays.length > 1 && props.onDisplayChange;
  const moreMenu = (
    <ToolbarMoreMenu
      {...props}
      menuRef={moreMenuRef}
      open={showMoreMenu}
      setOpen={setShowMoreMenu}
      transfer={transfer}
      panelStyle={
        isOverlayLayout
          ? moreMenuPanelStyle
          : { ...moreMenuPanelStyle, bottom: "calc(100% + 8px)" }
      }
    />
  );
  const primaryControls = (
    <>
      <ToolbarPointerLock {...props} />
      <ToolbarViewOnly {...props} />
      <ToolbarRecording {...props} />
      <ToolbarScreenshot {...props} />
      <ToolbarAudio {...props} />
    </>
  );

  if (!isOverlayLayout) {
    return (
      <div
        ref={toolbarRef}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 3,
          padding: "5px 12px",
          background:
            "linear-gradient(180deg, rgba(15, 15, 22, 0.85) 0%, rgba(8, 8, 14, 0.92) 100%)",
          backdropFilter: "blur(20px) saturate(1.4)",
          WebkitBackdropFilter: "blur(20px) saturate(1.4)",
          borderTop: "1px solid rgba(255,255,255,0.05)",
          boxShadow: "0 -4px 24px rgba(0,0,0,0.3)",
        }}
      >
        <ToolbarStatus {...props} isOverlayLayout={false} />
        <div style={dividerStyle} />
        <ToolbarQuality {...props} />
        <div style={dividerStyle} />
        <ToolbarScaling {...props} />
        <div style={dividerStyle} />
        {primaryControls}
        <div style={dividerStyle} />
        {moreMenu}
        <div style={dividerStyle} />
        <ToolbarFullscreen {...props} />
        <div style={dividerStyle} />
        <ToolbarDisconnect {...props} />
      </div>
    );
  }

  const revealHandle = autoHideEnabled ? (
    <ToolbarRevealHandle style={overlayHandleStyle} resetTimer={resetTimer} />
  ) : null;
  return (
    <div ref={toolbarRef} style={overlayContainerStyle}>
      {overlayPosition === "bottom" && revealHandle}
      <div style={overlayBodyStyle}>
        <ToolbarStatus {...props} isOverlayLayout />
        <ToolbarStatusBadges {...props} />
        <IconButton
          onClick={() => {
            setOverlayPosition((current) =>
              current === "top" ? "bottom" : "top",
            );
            setVisible(true);
          }}
          active={overlayPosition === "top"}
          title={
            overlayPosition === "top"
              ? "Move toolbar to bottom"
              : "Move toolbar to top"
          }
        >
          {overlayPosition === "top" ? (
            <ChevronsDown className="w-3.5 h-3.5" />
          ) : (
            <ChevronsUp className="w-3.5 h-3.5" />
          )}
        </IconButton>
        <IconButton
          onClick={() => {
            setAutoHideEnabled((current) => !current);
            setVisible(true);
          }}
          active={autoHideEnabled}
          title={autoHideEnabled ? "Disable auto-hide" : "Enable auto-hide"}
        >
          <Clock3 className="w-3.5 h-3.5" />
        </IconButton>
        <div style={dividerStyle} />
        <ToolbarQuality {...props} />
        {displayAvailable && (
          <>
            <div style={dividerStyle} />
            <ToolbarDisplayPicker {...props} />
          </>
        )}
        <div style={dividerStyle} />
        <ToolbarScaling {...props} />
        <div style={dividerStyle} />
        {primaryControls}
        <div style={dividerStyle} />
        {moreMenu}
        <ToolbarFullscreen {...props} />
        <div style={dividerStyle} />
        <ToolbarDisconnect {...props} />
      </div>
      {overlayPosition === "top" && revealHandle}
    </div>
  );
}
