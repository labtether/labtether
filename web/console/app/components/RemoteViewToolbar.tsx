"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Image as ImageIcon, Maximize, Minimize } from "lucide-react";
import type { RemoteViewToolbarProps } from "./remote-toolbar/types";
import { ToolButton } from "./remote-toolbar/ToolbarButtons";
import { ToolbarStatus } from "./remote-toolbar/ToolbarStatus";
import { ToolbarKeysMenu } from "./remote-toolbar/ToolbarKeysMenu";
import { ToolbarClipboard, ToolbarFiles, useToolbarTransferState } from "./remote-toolbar/ToolbarTransferControls";
import { ToolbarMoreMenu } from "./remote-toolbar/ToolbarMoreMenu";
import { ToolbarRevealHandle } from "./remote-toolbar/ToolbarRevealHandle";
import { useToolbarAutoHide } from "./remote-toolbar/useToolbarAutoHide";

export type { RemoteViewToolbarProps, RemoteViewToolbarLayout, ScalingMode } from "./remote-toolbar/types";

/**
 * The Remote View tools: how the screen is reached, then Send keys, Clipboard, Files, Screenshot, Full screen and More,
 * then Disconnect. Docked, they sit in the row above the screen. In full screen they float as a pill over the screen,
 * hide into a handle when the pointer rests, and can move to the bottom edge. Nothing shows until the screen is connected.
 */
export function RemoteViewToolbar(props: RemoteViewToolbarProps) {
  const t = useTranslations("desktop.tools");
  const { layout = "overlay", connectionState } = props;
  const overlay = layout === "overlay";
  const transfer = useToolbarTransferState(props.clipboardLastSync);
  const [openMenus, setOpenMenus] = useState(0);
  const [position, setPosition] = useState<"top" | "bottom">("top");
  const [autoHide, setAutoHide] = useState(true);
  const toolbarRef = useRef<HTMLDivElement>(null);
  const { visible, reveal } = useToolbarAutoHide({
    enabled: overlay && autoHide && connectionState === "connected",
    busy: openMenus > 0,
    position,
    toolbarRef,
  });

  if (connectionState !== "connected") return null;

  const onOpenChange = (open: boolean) => {
    setOpenMenus((n) => Math.max(0, n + (open ? 1 : -1)));
    if (open) reveal();
  };

  const body = (
    <div
      className={`rv-tools${overlay ? " pill" : ""}`}
      role="toolbar"
      aria-label={t("label")}
      aria-hidden={!visible || undefined}
      inert={!visible || undefined}
    >
      <ToolbarStatus {...props} clipboardFlash={transfer.clipboardFlash} pill={overlay} />
      <i className="rv-sep" aria-hidden="true" />
      <ToolbarKeysMenu {...props} onOpenChange={onOpenChange} />
      <ToolbarClipboard {...props} transfer={transfer} onOpenChange={onOpenChange} />
      <ToolbarFiles {...props} transfer={transfer} onOpenChange={onOpenChange} />
      {props.onScreenshot ? (
        <ToolButton label={t("screenshot")} onClick={props.onScreenshot}>
          <ImageIcon aria-hidden="true" />
        </ToolButton>
      ) : null}
      <ToolButton label={t(props.isFullscreen ? "exitFullScreen" : "fullScreen")} onClick={props.onFullscreenToggle}>
        {props.isFullscreen ? <Minimize aria-hidden="true" /> : <Maximize aria-hidden="true" />}
      </ToolButton>
      <ToolbarMoreMenu
        {...props}
        onOpenChange={onOpenChange}
        overlay={overlay}
        autoHide={autoHide}
        onAutoHideToggle={() => {
          setAutoHide((on) => !on);
          reveal();
        }}
        position={position}
        onPositionToggle={() => {
          setPosition((p) => (p === "top" ? "bottom" : "top"));
          reveal();
        }}
      />
      <i className="rv-sep" aria-hidden="true" />
      <button type="button" className="rv-disc" onClick={props.onDisconnect} title={t("disconnect")}>
        {t("disconnect")}
      </button>
    </div>
  );

  if (!overlay) return body;
  return (
    <div ref={toolbarRef} className={`rv-float ${position}${visible ? "" : " hidden"}`}>
      {body}
      {!visible ? <ToolbarRevealHandle position={position} onReveal={reveal} /> : null}
    </div>
  );
}
