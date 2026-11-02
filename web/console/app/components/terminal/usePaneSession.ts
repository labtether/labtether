"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useSession, type QuickConnectParams } from "../../hooks/useSession";
import type { TerminalClipboardActionResult, XTerminalHandle } from "../XTerminal";

export type PaneStatus = {
  /** "connecting" covers creating the session, authenticating and starting the shell. */
  state: "empty" | "idle" | "connecting" | "connected" | "error" | "disconnected";
  /** The hub's progress message while connecting ("Starting remote shell…") and how long it has taken. */
  progress: string;
  error: string;
  /** How the shell is reached: "agent", "proxmox", "ssh". */
  via: "agent" | "proxmox" | "ssh";
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
 * the focused pane's shortcuts (⌘/Ctrl+F find, ⌘/Ctrl+A select all, Ctrl+Shift+C copy, Ctrl+Shift+V paste).
 */
export function usePaneSession({
  targetNodeId, targetSource, isTabActive, isFocused, autoReconnect, quickConnectParams, onClipboardBlocked, onToggleSearch,
}: Options) {
  const session = useSession({ type: "terminal", autoReconnect: quickConnectParams ? false : autoReconnect, quickConnectParams });
  const termRef = useRef<XTerminalHandle | null>(null);
  const [wasConnected, setWasConnected] = useState(false);
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
    void connectRef.current("quick-connect");
  }, [quickConnectParams]);

  const handleConnected = useCallback(() => {
    setWasConnected(true);
    session.handleConnected();
  }, [session]);

  const handleStreamReady = useCallback((message?: string) => {
    setWasConnected(true);
    session.handleStreamReady(message);
  }, [session]);

  const reconnect = useCallback(() => {
    if (quickConnectParams) void session.connect("quick-connect");
    else if (targetNodeId) void session.connect(targetNodeId);
  }, [quickConnectParams, session, targetNodeId]);

  /** Switch the pane to another device: the old shell closes and the new one opens. */
  const switchTarget = useCallback((id: string) => {
    session.disconnect();
    setWasConnected(false);
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
      if (isEditableTarget(event.target)) return;
      if (!(event.ctrlKey || event.metaKey)) return;
      const key = event.key.toLowerCase();
      if (key === "f" && !event.shiftKey) {
        event.preventDefault();
        toggleSearchRef.current();
      } else if (key === "a" && !event.shiftKey) {
        event.preventDefault();
        termRef.current?.selectAll();
      } else if (event.shiftKey && key === "c") {
        event.preventDefault();
        void termRef.current?.copySelection().then((result) => report(result, "copy"));
      } else if (event.shiftKey && key === "v") {
        event.preventDefault();
        void termRef.current?.pasteFromClipboard().then((result) => report(result, "paste"));
      }
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
    progress: `${session.connectionProgress.message} (${formatElapsed(session.connectionProgress.totalElapsedMs)})`,
    error: session.error ?? "",
    via: quickConnectParams ? "ssh" : session.connectedAgentIds.has(targetNodeId) ? "agent" : targetSource === "proxmox" ? "proxmox" : "ssh",
  };

  return { session, termRef, status, handleConnected, handleStreamReady, reconnect, switchTarget, report };
}
