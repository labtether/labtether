"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useSession, type QuickConnectParams } from "../../hooks/useSession";
import type { TerminalClipboardActionResult, XTerminalHandle } from "../XTerminal";
import { isDeviceGone, isShellEnded, type TerminalClose } from "./terminalClose";
import { useTerminalWords } from "./terminalWords";
import { isShellInput, paneShortcut } from "./consoleShortcuts";

export type PaneStatus = {
  /** "connecting" covers creating the session, authenticating and starting the shell. */
  state: "empty" | "idle" | "connecting" | "connected" | "error" | "disconnected";
  /** The hub's progress message while connecting ("Starting remote shell…") and how long it has taken. */
  progress: string;
  error: string;
  /** How the shell is reached: "agent", "proxmox", "ssh". */
  via: "agent" | "proxmox" | "ssh";
  /**
   * Why a shell that was running stopped: "shell" when it ended (someone typed `exit`), "device" when the device
   * went away, "deviceStillAway" when Reconnect was pressed after that and the device had not come back. "" when
   * the pane does not know, or the shell has not stopped.
   */
  ended: "" | "shell" | "device" | "deviceStillAway";
};

type Options = {
  targetNodeId: string;
  targetSource: string;
  isTabActive: boolean;
  isFocused: boolean;
  autoReconnect: boolean;
  quickConnectParams?: QuickConnectParams;
  onClipboardBlocked: (action: "copy" | "paste") => void;
  onToggleSearch: () => void;
};

function formatElapsed(ms: number): string {
  const safe = Number.isFinite(ms) ? Math.max(0, ms) : 0;
  if (safe < 1000) return `${safe}ms`;
  const seconds = safe / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  return `${Math.floor(seconds / 60)}m ${Math.floor(seconds % 60)}s`;
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

/**
 * One pane's shell: connects when its tab is shown and it has a device (or an address), reconnects on request, and owns
 * the focused pane's shortcuts (Ctrl+Shift+C copy, ⌘F find and ⌘A select all on a Mac; while someone types in the
 * shell, Ctrl+A, Ctrl+F and paste stay with the shell and the browser).
 */
export function usePaneSession({
  targetNodeId, targetSource, isTabActive, isFocused, autoReconnect, quickConnectParams, onClipboardBlocked, onToggleSearch,
}: Options) {
  const session = useSession({ type: "terminal", autoReconnect: quickConnectParams ? false : autoReconnect, quickConnectParams });
  const say = useTerminalWords();
  const termRef = useRef<XTerminalHandle | null>(null);
  const [wasConnected, setWasConnected] = useState(false);
  const [ended, setEnded] = useState<PaneStatus["ended"]>("");
  const shellRunningRef = useRef(false);
  const setTargetRef = useRef(session.setTarget);
  const connectRef = useRef(session.connect);
  setTargetRef.current = session.setTarget;
  connectRef.current = session.connect;

  useEffect(() => {
    if (targetNodeId) setTargetRef.current(targetNodeId);
  }, [targetNodeId]);

  // Connect by itself once per device while the tab shows. A stream that never opens (a proxy without websocket
  // upgrade, a console address the Hub does not allow) then ends in the error with Try again. Before, the pane went
  // back to idle and connected again at once, a few times a second, for as long as the tab stayed open.
  const autoTriedRef = useRef<string | null>(null);
  useEffect(() => {
    if (!isTabActive) {
      autoTriedRef.current = null;
      return;
    }
    if (targetNodeId && !quickConnectParams && session.connectionState === "idle" && !wasConnected && autoTriedRef.current !== targetNodeId) {
      autoTriedRef.current = targetNodeId;
      void connectRef.current(targetNodeId);
    }
  }, [isTabActive, targetNodeId, quickConnectParams, session.connectionState, wasConnected]);

  // A new address from the page's form: connect to it once its parameters have settled into useSession.
  useEffect(() => {
    if (!quickConnectParams) return;
    setWasConnected(false);
    setEnded("");
    void connectRef.current("quick-connect");
  }, [quickConnectParams]);

  const handleConnected = useCallback(() => {
    setWasConnected(true);
    session.handleConnected();
  }, [session]);

  const handleStreamReady = useCallback((message?: string) => {
    setWasConnected(true);
    shellRunningRef.current = true;
    setEnded("");
    session.handleStreamReady(message);
  }, [session]);

  // The stream closed. Only a shell that was running can end or lose its device. A stream that closes before its
  // shell starts is a Reconnect that failed: after a lost device, the device is still away.
  const handleDisconnected = useCallback((close: TerminalClose) => {
    const wasRunning = shellRunningRef.current;
    shellRunningRef.current = false;
    setEnded((before) => {
      if (wasRunning) return isShellEnded(close) ? "shell" : isDeviceGone(close) ? "device" : "";
      return before === "device" ? "deviceStillAway" : before;
    });
    session.handleDisconnected(close);
  }, [session]);

  const reconnect = useCallback(() => {
    shellRunningRef.current = false;
    if (quickConnectParams) void session.connect("quick-connect");
    else if (targetNodeId) void session.connect(targetNodeId);
  }, [quickConnectParams, session, targetNodeId]);

  /** Switch the pane to another device: the old shell closes and the new one opens. */
  const switchTarget = useCallback((id: string) => {
    session.disconnect();
    setWasConnected(false);
    shellRunningRef.current = false;
    setEnded("");
    void session.connect(id);
  }, [session]);

  const report = useCallback((result: TerminalClipboardActionResult | undefined, action: "copy" | "paste") => {
    if (result && !result.ok && result.reason === "permission-denied") onClipboardBlocked(action);
  }, [onClipboardBlocked]);

  const toggleSearchRef = useRef(onToggleSearch);
  toggleSearchRef.current = onToggleSearch;

  useEffect(() => {
    if (!isFocused) return undefined;
    const handler = (event: KeyboardEvent) => {
      // xterm's input is the shell, not a form field: the pane's keys count there too (see consoleShortcuts.ts).
      const inShell = isShellInput(event.target);
      if (!inShell && isEditableTarget(event.target)) return;
      const action = paneShortcut(event, inShell);
      if (!action) return;
      event.preventDefault();
      if (action === "find") toggleSearchRef.current();
      else if (action === "selectAll") termRef.current?.selectAll();
      else if (action === "copy") void termRef.current?.copySelection().then((result) => report(result, "copy"));
      else void termRef.current?.pasteFromClipboard().then((result) => report(result, "paste"));
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [isFocused, report]);

  const state: PaneStatus["state"] = !targetNodeId && !quickConnectParams
    ? "empty"
    : session.connectionState === "connected"
      ? "connected"
      : session.connectionState === "connecting" || session.connectionState === "authenticating"
        ? "connecting"
        : session.connectionState === "error"
          ? "error"
          : wasConnected
            ? "disconnected"
            : session.error
              ? "error"
              : "idle";
  const status: PaneStatus = {
    state,
    progress: `${say(session.connectionProgress.message)} (${formatElapsed(session.connectionProgress.totalElapsedMs)})`,
    error: session.error ? say(session.error) : "",
    via: quickConnectParams ? "ssh" : session.connectedAgentIds.has(targetNodeId) ? "agent" : targetSource === "proxmox" ? "proxmox" : "ssh",
    // Once the Hub sees the device again, the pane no longer says it is still away.
    ended: ended === "deviceStillAway" && session.connectedAgentIds.has(targetNodeId) ? "device" : ended,
  };

  return { session, termRef, status, handleConnected, handleStreamReady, handleDisconnected, reconnect, switchTarget, report };
}
