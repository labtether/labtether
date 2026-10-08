"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent, type RefObject } from "react";
import { useSession } from "../../hooks/useSession";
import type { QuickConnectParams } from "../../hooks/useSession";
import { useFastStatus } from "../../contexts/StatusContext";
import XTerminal from "../XTerminal";
import SearchBar from "./SearchBar";
import ConnectionLanding from "./ConnectionLanding";
import QuickConnectDialog from "./QuickConnectDialog";
import type { TerminalClipboardActionResult, XTerminalHandle } from "../XTerminal";
import type { TerminalPreferences } from "../../hooks/useTerminalPreferences";
import type { TerminalThemeDef } from "../../terminal/themes";
import type { TerminalFontDef } from "../../terminal/fonts";
import { isHiddenAsset } from "../../console/taxonomy";
import { RotateCcw, Wifi, WifiOff } from "lucide-react";
import { TerminalTargetPicker } from "./TerminalTargetPicker";
import { TerminalPaneMenu, type PaneContextMenuState } from "./TerminalPaneMenu";
import { saveRecentTarget } from "./terminalRecentTargets";
export { loadRecentTargets, saveRecentTarget, recentsStorageKey } from "./terminalRecentTargets";
export type { RecentTarget } from "./terminalRecentTargets";

interface TerminalPaneProps {
  paneIndex: number;
  targetNodeId: string;
  onTargetChange: (nodeId: string) => void;
  isTabActive: boolean;
  isFocused: boolean;
  onFocus: () => void;
  broadcastActive: boolean;
  onBroadcastData: (data: string, sourcePaneIndex: number) => void;
  onTerminalRef: (handle: XTerminalHandle | null) => void;
  prefs: TerminalPreferences;
  themeDef: TerminalThemeDef;
  fontDef: TerminalFontDef;
  /** Optional: bypass target selection and attach to an existing persistent session. */
  initialPersistentSessionId?: string;
  /** Optional: pre-issued stream ticket for direct WebSocket connection. */
  initialStreamTicket?: string;
}

type ClipboardNotice = {
  id: number;
  message: string;
};

function clampMenuPosition(
  x: number,
  y: number,
  bounds?: { width: number; height: number },
  width = 220,
  height = 280,
  padding = 8,
) {
  const maxWidth = bounds?.width ?? (typeof window !== "undefined" ? window.innerWidth : width + padding * 2);
  const maxHeight = bounds?.height ?? (typeof window !== "undefined" ? window.innerHeight : height + padding * 2);
  return {
    x: Math.max(padding, Math.min(x, maxWidth - width - padding)),
    y: Math.max(padding, Math.min(y, maxHeight - height - padding)),
  };
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

function formatElapsed(ms: number): string {
  const safe = Number.isFinite(ms) ? Math.max(0, ms) : 0;
  if (safe < 1000) return `${safe}ms`;
  const seconds = safe / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const rem = Math.floor(seconds % 60);
  return `${minutes}m ${rem}s`;
}

export default function TerminalPane({
  paneIndex,
  targetNodeId,
  onTargetChange,
  isTabActive,
  isFocused,
  onFocus,
  broadcastActive,
  onBroadcastData,
  onTerminalRef,
  prefs,
  themeDef,
  fontDef,
  initialPersistentSessionId: _initialPersistentSessionId,
  initialStreamTicket: _initialStreamTicket,
}: TerminalPaneProps) {
  const [quickConnectParams, setQuickConnectParams] = useState<QuickConnectParams | undefined>(undefined);
  const [quickConnectOpen, setQuickConnectOpen] = useState(false);
  const session = useSession({ type: "terminal", autoReconnect: quickConnectParams ? false : prefs.auto_reconnect, quickConnectParams });
  const status = useFastStatus();
  const termRef = useRef<XTerminalHandle | null>(null);
  const paneMenuHostRef = useRef<HTMLDivElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [wasConnected, setWasConnected] = useState(false);
  const [targetPickerOpen, setTargetPickerOpen] = useState(false);
  const [paneMenu, setPaneMenu] = useState<PaneContextMenuState | null>(null);
  const [clipboardNotice, setClipboardNotice] = useState<ClipboardNotice | null>(null);
  const setTargetRef = useRef(session.setTarget);
  const connectRef = useRef(session.connect);

  const assets = status?.assets;
  const visibleTargets = useMemo(
    () => (assets ?? []).filter((asset) => !isHiddenAsset(asset)),
    [assets],
  );

  useEffect(() => {
    setTargetRef.current = session.setTarget;
  }, [session.setTarget]);

  useEffect(() => {
    connectRef.current = session.connect;
  }, [session.connect]);

  useEffect(() => {
    if (targetNodeId) {
      setTargetRef.current(targetNodeId);
    }
  }, [targetNodeId]);

  useEffect(() => {
    if (isTabActive && targetNodeId && session.connectionState === "idle" && !wasConnected) {
      void connectRef.current(targetNodeId);
    }
  }, [isTabActive, targetNodeId, session.connectionState, wasConnected]);

  const handleConnected = useCallback(() => {
    setWasConnected(true);
    session.handleConnected();
  }, [session]);

  const handleStreamReady = useCallback((message?: string) => {
    setWasConnected(true);
    session.handleStreamReady(message);
  }, [session]);

  const handleDisconnected = useCallback(
    (reason?: string) => {
      session.handleDisconnected(reason);
    },
    [session],
  );

  // Forward ref to parent.
  const setTermRef = useCallback(
    (el: XTerminalHandle | null) => {
      termRef.current = el;
      onTerminalRef(el);
    },
    [onTerminalRef],
  );

  const closePaneMenu = useCallback(() => {
    setPaneMenu(null);
  }, []);

  useEffect(() => {
    if (!clipboardNotice) return undefined;
    const timer = window.setTimeout(() => setClipboardNotice(null), 4200);
    return () => window.clearTimeout(timer);
  }, [clipboardNotice]);

  const handleClipboardActionResult = useCallback(
    (result: TerminalClipboardActionResult, action: "copy" | "paste") => {
      if (result.ok || result.reason !== "permission-denied") {
        return;
      }
      const message = action === "copy"
        ? "Clipboard write was blocked by the browser. Allow clipboard access or use your browser's native copy prompt."
        : "Clipboard read was blocked by the browser. Allow clipboard access and try paste again.";
      setClipboardNotice({ id: Date.now(), message });
    },
    [],
  );

  // Broadcast data capture: forward typed data to other panes.
  const handleDataCapture = useCallback(
    (data: string) => {
      if (broadcastActive && isFocused) {
        onBroadcastData(data, paneIndex);
      }
    },
    [broadcastActive, isFocused, onBroadcastData, paneIndex],
  );

  // Keyboard shortcuts for focused pane.
  useEffect(() => {
    if (!isFocused) return undefined;
    const handler = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) return;
      const key = event.key.toLowerCase();
      const hasPrimaryModifier = event.ctrlKey || event.metaKey;

      if (!hasPrimaryModifier) return;

      if (key === "f") {
        event.preventDefault();
        setSearchOpen((value) => !value);
        return;
      }

      if (key === "a" && !event.shiftKey) {
        event.preventDefault();
        termRef.current?.selectAll();
        return;
      }

      if (event.shiftKey && key === "c") {
        event.preventDefault();
        void termRef.current?.copySelection().then((result) => {
          if (result) handleClipboardActionResult(result, "copy");
        });
        return;
      }

      if (event.shiftKey && key === "v") {
        event.preventDefault();
        void termRef.current?.pasteFromClipboard().then((result) => {
          if (result) handleClipboardActionResult(result, "paste");
        });
      }
    };

    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [handleClipboardActionResult, isFocused]);

  useEffect(() => {
    if (!paneMenu) return undefined;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closePaneMenu();
      }
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [paneMenu, closePaneMenu]);

  const handleTargetSelect = useCallback(
    (newTarget: string) => {
      const trimmed = newTarget.trim();
      if (!trimmed || trimmed === targetNodeId) {
        return;
      }
      onTargetChange(trimmed);
      session.disconnect();
      setWasConnected(false);
      setTargetPickerOpen(false);
      void session.connect(trimmed);
    },
    [onTargetChange, session, targetNodeId],
  );

  const handleReconnect = useCallback(() => {
    if (targetNodeId) {
      void session.connect(targetNodeId);
    }
  }, [session, targetNodeId]);

  const handleQuickConnect = useCallback(
    (params: QuickConnectParams) => {
      setQuickConnectOpen(false);
      session.disconnect();
      setWasConnected(false);
      setQuickConnectParams(params);
      // connect() is triggered by useEffect below once quickConnectParams state settles.
    },
    [session],
  );

  // Trigger connect once when quickConnectParams are set.
  // Intentionally excludes session.connect from deps — we only want to fire
  // when quickConnectParams changes, not on every session state transition.
  const sessionConnectRef = useRef(session.connect);
  sessionConnectRef.current = session.connect;

  useEffect(() => {
    if (!quickConnectParams) return;
    void sessionConnectRef.current("quick-connect");
  }, [quickConnectParams]);

  const handlePaneContextMenu = useCallback((event: MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();
    onFocus();
    const hostRect = paneMenuHostRef.current?.getBoundingClientRect();
    const pos = clampMenuPosition(
      hostRect ? event.clientX - hostRect.left : event.clientX,
      hostRect ? event.clientY - hostRect.top : event.clientY,
      hostRect ? { width: hostRect.width, height: hostRect.height } : undefined,
    );
    setPaneMenu({
      x: pos.x,
      y: pos.y,
      hasSelection: termRef.current?.hasSelection() ?? false,
    });
  }, [onFocus]);

  const handleCopySelection = useCallback(async () => {
    const result = await termRef.current?.copySelection();
    if (result) handleClipboardActionResult(result, "copy");
    closePaneMenu();
    termRef.current?.focus();
  }, [closePaneMenu, handleClipboardActionResult]);

  const handlePaste = useCallback(async () => {
    const result = await termRef.current?.pasteFromClipboard();
    if (result) handleClipboardActionResult(result, "paste");
    closePaneMenu();
    termRef.current?.focus();
  }, [closePaneMenu, handleClipboardActionResult]);

  const handleSelectAll = useCallback(() => {
    termRef.current?.selectAll();
    closePaneMenu();
  }, [closePaneMenu]);

  const handleOpenSearch = useCallback(() => {
    setSearchOpen(true);
    closePaneMenu();
  }, [closePaneMenu]);

  const handleClearScrollback = useCallback(() => {
    termRef.current?.clearScrollback();
    closePaneMenu();
    termRef.current?.focus();
  }, [closePaneMenu]);

  const handleDisconnect = useCallback(() => {
    session.disconnect();
    closePaneMenu();
  }, [session, closePaneMenu]);

  const handleReconnectFromMenu = useCallback(() => {
    handleReconnect();
    closePaneMenu();
  }, [handleReconnect, closePaneMenu]);

  const isConnected = session.connectionState === "connected";
  const isDisconnected = session.connectionState === "idle" && wasConnected;
  const isError = session.connectionState === "error";

  const paneBorder = isFocused
    ? broadcastActive
      ? "1px solid var(--warn)"
      : "1px solid var(--accent)"
    : broadcastActive
      ? "1px solid var(--warn-glow)"
      : "1px solid var(--line)";

  const statusColor = isConnected
    ? "var(--ok)"
    : session.connectionState === "connecting"
      ? "var(--warn)"
      : isError
        ? "var(--bad)"
        : "var(--muted)";

  const showProgress = session.connectionState === "connecting" || session.connectionState === "authenticating";
  const progressLabel = `${session.connectionProgress.message} (${formatElapsed(session.connectionProgress.totalElapsedMs)})`;

  return (
    <div
      ref={paneMenuHostRef}
      onClick={onFocus}
      className="relative flex h-full min-h-0 flex-col overflow-hidden rounded-lg bg-[var(--panel)]"
      style={{ border: paneBorder }}
    >
      {broadcastActive ? (
        <div
          className="absolute left-0 right-0 top-0 z-[5] h-0.5"
          style={{ backgroundColor: "var(--warn)" }}
        />
      ) : null}

      {/* Pane header */}
      <div className="flex shrink-0 items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-2 py-1.5 text-xs">
        <TerminalTargetPicker
          assets={visibleTargets}
          selectedTargetID={targetNodeId}
          connectedAgentIDs={session.connectedAgentIds}
          onSelect={handleTargetSelect}
          open={targetPickerOpen}
          onOpenChange={setTargetPickerOpen}
        />

        <span
          className="inline-block h-2 w-2 rounded-full"
          style={{ backgroundColor: statusColor }}
          title={session.connectionState}
        />

        {broadcastActive ? (
          <span className="rounded-full border border-[var(--warn)]/45 bg-[var(--warn-glow)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--warn)]">
            Cast
          </span>
        ) : null}

        {showProgress ? (
          <span className="rounded-full border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-[10px] uppercase tracking-[0.06em] text-[var(--muted)]">
            {progressLabel}
          </span>
        ) : null}

        {isConnected ? (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              session.disconnect();
            }}
            className="inline-flex h-5 w-5 items-center justify-center rounded border border-transparent text-[var(--muted)] transition-colors hover:border-[var(--line)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
            title="Disconnect"
          >
            <WifiOff size={11} />
          </button>
        ) : null}
      </div>

      {/* Terminal area */}
      <div
        className="relative min-h-0 flex-1"
        onContextMenu={handlePaneContextMenu}
      >
        <SearchBar
          termRef={termRef as RefObject<XTerminalHandle | null>}
          open={searchOpen}
          onClose={() => setSearchOpen(false)}
        />

        {session.wsUrl ? (
          <>
            <XTerminal
              ref={setTermRef}
              wsUrl={session.wsUrl}
              onConnected={handleConnected}
              onDisconnected={handleDisconnected}
              onError={session.handleError}
              onStreamStatus={session.handleStreamStatus}
              onStreamReady={handleStreamReady}
              theme={themeDef.theme}
              fontFamily={fontDef.family}
              fontSize={prefs.font_size}
              cursorStyle={prefs.cursor_style}
              cursorBlink={prefs.cursor_blink}
              scrollback={prefs.scrollback}
              onDataCapture={handleDataCapture}
            />
            {showProgress ? (
              <div className="pointer-events-none absolute left-3 top-3 z-[6] rounded-md border border-[var(--line)] bg-[var(--panel)] px-2 py-1 text-[10px] text-[var(--muted)] backdrop-blur-sm">
                {progressLabel}
              </div>
            ) : null}
          </>
        ) : (
          <div className="flex h-full min-h-[120px] flex-col items-center justify-center gap-3 px-3 text-center text-xs text-[var(--muted)]">
            {!targetNodeId && !quickConnectParams ? (
              <ConnectionLanding
                onSelectTarget={handleTargetSelect}
                onBrowseTargets={() => setTargetPickerOpen(true)}
                onQuickConnect={() => setQuickConnectOpen(true)}
              />
            ) : session.connectionState === "connecting" ? (
              <span>{progressLabel}</span>
            ) : isError ? (
              <>
                <span className="text-[var(--bad)]">{session.error || "Connection error"}</span>
                <button
                  type="button"
                  onClick={handleReconnect}
                  className="inline-flex items-center gap-1.5 rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
                >
                  <RotateCcw size={12} />
                  Retry
                </button>
              </>
            ) : targetNodeId && !wasConnected ? (
              <button
                type="button"
                onClick={handleReconnect}
                className="inline-flex items-center gap-1.5 rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
              >
                <Wifi size={12} />
                Connect
              </button>
            ) : null}
          </div>
        )}

        {/* Reconnect overlay */}
        {isDisconnected ? (
          <div className="absolute inset-0 z-[5] flex flex-col items-center justify-center gap-3 bg-black/65 px-3">
            <span className="text-xs text-[var(--text)]">Session disconnected</span>
            <button
              type="button"
              onClick={handleReconnect}
              className="inline-flex items-center gap-1.5 rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
            >
              <RotateCcw size={12} />
              Reconnect
            </button>
          </div>
        ) : null}

        {clipboardNotice ? (
          <div className="pointer-events-none absolute right-3 top-3 z-[7] max-w-[20rem]">
            <div
              key={clipboardNotice.id}
              role="status"
              aria-live="polite"
              className="rounded-md border border-[var(--warn)]/35 bg-[var(--panel)]/95 px-3 py-2 text-xs text-[var(--warn)] shadow-[var(--shadow-md)] backdrop-blur-sm"
            >
              {clipboardNotice.message}
            </div>
          </div>
        ) : null}

      </div>

      {paneMenu ? (
        <TerminalPaneMenu
          paneMenu={paneMenu}
          isConnected={isConnected}
          canReconnect={Boolean(targetNodeId)}
          closePaneMenu={closePaneMenu}
          handleCopySelection={handleCopySelection}
          handlePaste={handlePaste}
          handleSelectAll={handleSelectAll}
          handleOpenSearch={handleOpenSearch}
          handleClearScrollback={handleClearScrollback}
          handleDisconnect={handleDisconnect}
          handleReconnectFromMenu={handleReconnectFromMenu}
        />
      ) : null}

      <QuickConnectDialog
        open={quickConnectOpen}
        onClose={() => setQuickConnectOpen(false)}
        onConnect={handleQuickConnect}
      />
    </div>
  );
}
