"use client";

import dynamic from "next/dynamic";
import type { CSSProperties, RefObject } from "react";
import { useTranslations } from "next-intl";
import type { ITheme } from "@xterm/xterm";
import type { SessionConnectionState, SessionStreamStatus, SessionType } from "../hooks/useSession";
import type { XTerminalHandle } from "./XTerminal";

/** While the terminal's code loads: a quiet empty frame, no words. */
const XTerminal = dynamic(() => import("./XTerminal"), {
  ssr: false,
  loading: () => <div className="tm-loading-frame" aria-busy="true" />,
});

/** Remote View's protocols (its viewers live in RemoteViewerShell; the types stay here for its callers). */
export type DesktopProtocol = "vnc" | "rdp" | "spice" | "webrtc";

export interface SpiceTicket {
  wsUrl: string;
  password: string;
  type?: string;
  ca?: string;
  proxy?: string;
}

export interface SessionPanelProps {
  /** Only "terminal" is drawn here; Remote View has its own viewer shell. */
  type: SessionType;
  connectionState: SessionConnectionState;
  wsUrl: string | null;
  error: string | null;
  target: string;
  /** Kept for callers; how the shell is reached shows in the toolbar. */
  connectedAgentIds: Set<string>;
  onRetry: () => void;
  termRef?: RefObject<XTerminalHandle | null>;
  onTerminalConnected?: () => void;
  onTerminalDisconnected?: (reason?: string) => void;
  onTerminalError?: (message: string) => void;
  onTerminalStreamReady?: (message?: string) => void;
  onTerminalStreamStatus?: (status: SessionStreamStatus) => void;
  terminalTheme?: ITheme;
  terminalFontFamily?: string;
  terminalFontSize?: number;
  terminalCursorStyle?: "block" | "underline" | "bar";
  terminalCursorBlink?: boolean;
  terminalScrollback?: number;
  /** Replaces "Not connected" while idle. */
  idleText?: string;
}

/**
 * One terminal on the 2.0 canvas: a red line with Retry when the hub reports an error, "Connecting to …" while the
 * shell starts, and "Not connected" with Connect while idle.
 */
export function SessionPanel({
  connectionState,
  wsUrl,
  error,
  target,
  onRetry,
  termRef,
  onTerminalConnected,
  onTerminalDisconnected,
  onTerminalError,
  onTerminalStreamReady,
  onTerminalStreamStatus,
  terminalTheme,
  terminalFontFamily,
  terminalFontSize,
  terminalCursorStyle,
  terminalCursorBlink,
  terminalScrollback,
  idleText,
}: SessionPanelProps) {
  const t = useTranslations("terminal.panel");
  const isConnecting = connectionState === "connecting" || connectionState === "authenticating";

  const overlay = isConnecting ? (
    <div className="tm-state over">
      <p className="tm-state-t">{t("connectingTo", { target })}</p>
    </div>
  ) : connectionState === "idle" && !wsUrl ? (
    <div className="tm-state over">
      <p className="tm-state-t">{idleText ?? t("notConnected")}</p>
      {target ? <button type="button" className="tm-tbtn" onClick={onRetry}>{t("connect")}</button> : null}
    </div>
  ) : null;

  const colours = terminalTheme?.background
    ? ({ "--tm-bg": terminalTheme.background, "--tm-fg": terminalTheme.foreground } as CSSProperties)
    : undefined;

  return (
    <>
      {error ? (
        <div className="tm-alert" role="alert">
          <span>{error}</span>
          <button type="button" className="tm-qbtn" onClick={onRetry} disabled={!target}>{t("retry")}</button>
        </div>
      ) : null}
      <div className="tm-canvas" style={colours}>
        <div className="tm-xt">
          <XTerminal
            ref={termRef}
            wsUrl={wsUrl}
            onConnected={onTerminalConnected}
            onDisconnected={onTerminalDisconnected}
            onError={onTerminalError}
            onStreamReady={onTerminalStreamReady}
            onStreamStatus={onTerminalStreamStatus}
            theme={terminalTheme}
            fontFamily={terminalFontFamily}
            fontSize={terminalFontSize}
            cursorStyle={terminalCursorStyle}
            cursorBlink={terminalCursorBlink}
            scrollback={terminalScrollback}
          />
        </div>
        {overlay}
      </div>
    </>
  );
}
