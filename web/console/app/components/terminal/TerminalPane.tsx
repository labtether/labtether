"use client";

import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import { useFastStatus } from "../../contexts/StatusContext";
import { isHiddenAsset } from "../../console/taxonomy";
import type { QuickConnectParams } from "../../hooks/useSession";
import type { TerminalPreferences } from "../../hooks/useTerminalPreferences";
import type { TerminalFontDef } from "../../terminal/fonts";
import type { TerminalThemeDef } from "../../terminal/themes";
import XTerminal, { type XTerminalHandle } from "../XTerminal";
import SearchBar from "./SearchBar";
import TerminalPicker from "./TerminalPicker";
import { TerminalPaneMenu, type PaneContextMenuState } from "./TerminalPaneMenu";
import { TerminalTargetPicker } from "./TerminalTargetPicker";
import { usePaneSession, type PaneStatus } from "./usePaneSession";
export { loadRecentTargets, saveRecentTarget, recentsStorageKey } from "./terminalRecentTargets";
export type { RecentTarget } from "./terminalRecentTargets";
export type { PaneStatus } from "./usePaneSession";

/** What the workspace bar and the ⋯ menu can do to a pane. */
export type PaneControls = {
  disconnect: () => void;
  reconnect: () => void;
  openFind: () => void;
  changeTarget: () => void;
  copyAll: () => void;
  clear: () => void;
};

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
  /** More than one pane shows: each gets a head with its name, status and Disconnect. */
  split?: boolean;
  /** An address from "Connect to an address…" for this pane (kept in memory only). */
  quickConnectParams?: QuickConnectParams;
  /** Opens the address form for this pane, filled with the address when given. */
  onRequestAddress?: (address: string) => void;
  onStatusChange?: (status: PaneStatus) => void;
  onControls?: (controls: PaneControls | null) => void;
  /** Optional: attach to an existing persistent session (not used yet). */
  initialPersistentSessionId?: string;
  initialStreamTicket?: string;
}

const clamp = (value: number, max: number, size: number) => Math.max(8, Math.min(value, max - size - 8));

/** One shell in the workspace, or the device list when the pane has no device yet. */
export default function TerminalPane({
  paneIndex, targetNodeId, onTargetChange, isTabActive, isFocused, onFocus, broadcastActive, onBroadcastData,
  onTerminalRef, prefs, themeDef, fontDef, split = false, quickConnectParams, onRequestAddress, onStatusChange, onControls,
}: TerminalPaneProps) {
  const t = useTranslations("terminal.pane");
  const tWords = useTranslations("terminal.words");
  const status = useFastStatus();
  const assets = useMemo(() => (status?.assets ?? []).filter((asset) => !isHiddenAsset(asset)), [status?.assets]);
  const asset = assets.find((entry) => entry.id === targetNodeId);
  const [searchOpen, setSearchOpen] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [paneMenu, setPaneMenu] = useState<PaneContextMenuState | null>(null);
  const [clipboardNotice, setClipboardNotice] = useState<{ id: number; action: "copy" | "paste" } | null>(null);
  const hostRef = useRef<HTMLElement | null>(null);

  const onClipboardBlocked = useCallback((action: "copy" | "paste") => setClipboardNotice({ id: Date.now(), action }), []);
  const toggleSearch = useCallback(() => setSearchOpen((open) => !open), []);
  const pane = usePaneSession({
    targetNodeId, targetSource: asset?.source ?? "", isTabActive, isFocused, autoReconnect: prefs.auto_reconnect,
    quickConnectParams, onClipboardBlocked, onToggleSearch: toggleSearch,
  });
  const { session, termRef } = pane;
  const name = quickConnectParams
    ? `${quickConnectParams.username}@${quickConnectParams.host}`
    : asset?.name || targetNodeId;

  useEffect(() => {
    if (!clipboardNotice) return undefined;
    const timer = window.setTimeout(() => setClipboardNotice(null), 4200);
    return () => window.clearTimeout(timer);
  }, [clipboardNotice]);

  useEffect(() => {
    if (!paneMenu) return undefined;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setPaneMenu(null);
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [paneMenu]);

  const onStatusRef = useRef(onStatusChange);
  onStatusRef.current = onStatusChange;
  const { state, progress, error, via, ended } = pane.status;
  // The bar above needs the state, not the ticking progress text.
  useEffect(() => {
    onStatusRef.current?.({ state, progress: "", error, via, ended });
  }, [state, error, via, ended]);

  const reconnectRef = useRef(pane.reconnect);
  reconnectRef.current = pane.reconnect;
  const disconnectRef = useRef(session.disconnect);
  disconnectRef.current = session.disconnect;
  const reportRef = useRef(pane.report);
  reportRef.current = pane.report;
  useEffect(() => {
    onControls?.({
      disconnect: () => disconnectRef.current(),
      reconnect: () => reconnectRef.current(),
      openFind: () => setSearchOpen(true),
      changeTarget: () => setPickerOpen(true),
      copyAll: () => {
        termRef.current?.selectAll();
        void termRef.current?.copySelection().then((result) => reportRef.current(result, "copy"));
      },
      clear: () => {
        termRef.current?.clearScrollback();
        termRef.current?.focus();
      },
    });
    return () => onControls?.(null);
  }, [onControls, termRef]);

  const setTermRef = useCallback((handle: XTerminalHandle | null) => {
    termRef.current = handle;
    onTerminalRef(handle);
  }, [onTerminalRef, termRef]);

  const handleTargetSelect = useCallback((id: string) => {
    const next = id.trim();
    if (!next || (next === targetNodeId && !quickConnectParams)) return;
    onTargetChange(next);
    pane.switchTarget(next);
  }, [onTargetChange, pane, quickConnectParams, targetNodeId]);

  const handleDataCapture = useCallback((data: string) => {
    if (broadcastActive && isFocused) onBroadcastData(data, paneIndex);
  }, [broadcastActive, isFocused, onBroadcastData, paneIndex]);

  const openMenu = useCallback((event: MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();
    onFocus();
    const rect = hostRef.current?.getBoundingClientRect();
    const x = rect ? event.clientX - rect.left : event.clientX;
    const y = rect ? event.clientY - rect.top : event.clientY;
    setPaneMenu({
      x: rect ? clamp(x, rect.width, 220) : x,
      y: rect ? clamp(y, rect.height, 300) : y,
      hasSelection: termRef.current?.hasSelection() ?? false,
    });
  }, [onFocus, termRef]);

  const closeMenuThen = (action: () => void) => () => {
    action();
    setPaneMenu(null);
  };
  const requestAddress = (address: string) => onRequestAddress?.(address);

  if (state === "empty") {
    return (
      <section className={`tm-pane slot${split && isFocused ? " on" : ""}`} aria-label={t("emptyPane")} onClick={onFocus}>
        <TerminalPicker
          assets={assets}
          connectedAgentIds={session.connectedAgentIds}
          onPick={handleTargetSelect}
          onAddress={requestAddress}
          autoFocus={isTabActive && isFocused}
          compact
        />
      </section>
    );
  }

  const live = state === "connected";
  const statusWord = live
    ? via === "agent" ? t("connected") : t("connectedVia", { via: t(`via_${via}`) })
    : state === "connecting" ? t("connecting") : state === "error" ? t("failed")
      : state === "disconnected" ? ended === "shell" ? tWords("sessionEnded") : t("disconnected") : "";
  // A shell that ended and a device that went away are different things: say which, and name the device.
  const stoppedText = ended === "shell" ? tWords("sessionEnded")
    : ended === "device" ? t("deviceDisconnected", { name })
      : ended === "deviceStillAway" ? t("deviceStillDisconnected", { name }) : t("sessionDisconnected");

  return (
    <section
      ref={hostRef}
      className={`tm-pane${split && isFocused ? " on" : ""}${broadcastActive ? " cast" : ""}`}
      style={{ "--tm-bg": themeDef.theme.background, "--tm-fg": themeDef.theme.foreground } as CSSProperties}
      aria-label={t("sessionOn", { name })}
      onClick={onFocus}
    >
      {split ? (
        <div className="tm-ph">
          {state === "error" || state === "disconnected" ? <i className="kt-dot sm bad" aria-hidden="true" /> : null}
          <button type="button" className="tm-ph-nm" title={t("changeDevice")} onClick={() => setPickerOpen(true)}>{name}</button>
          <span className="tm-ph-st">{statusWord}</span>
          {broadcastActive ? <span className="tm-ph-all">{t("sendingToAll")}</span> : null}
          <span className="tm-sp" />
          {live ? (
            <button type="button" className="tm-ph-btn" aria-label={t("disconnectFrom", { name })} onClick={() => session.disconnect()}>
              {t("disconnect")}
            </button>
          ) : null}
        </div>
      ) : null}

      <div className="tm-body" onContextMenu={openMenu}>
        <SearchBar termRef={termRef} open={searchOpen} onClose={() => setSearchOpen(false)} />
        {session.wsUrl ? (
          <div className="tm-xt">
            <XTerminal
              ref={setTermRef}
              wsUrl={session.wsUrl}
              endedText={tWords("sessionEnded")}
              deviceGoneText={t("deviceDisconnected", { name })}
              onConnected={pane.handleConnected}
              onDisconnected={pane.handleDisconnected}
              onError={session.handleError}
              onStreamStatus={session.handleStreamStatus}
              onStreamReady={pane.handleStreamReady}
              theme={themeDef.theme}
              fontFamily={fontDef.family}
              fontSize={prefs.font_size}
              cursorStyle={prefs.cursor_style}
              cursorBlink={prefs.cursor_blink}
              scrollback={prefs.scrollback}
              onDataCapture={handleDataCapture}
            />
          </div>
        ) : null}
        {state === "connecting" ? <p className="tm-progress">{progress}</p> : null}
        {/* A stream that fails keeps its ended terminal mounted (wsUrl stays set), so the error covers it. */}
        {state === "error" ? (
          <div className={`tm-state${session.wsUrl ? " over" : ""}`}>
            <p className="tm-state-t bad">{error || t("failed")}</p>
            <button type="button" className="tm-tbtn" onClick={pane.reconnect}>{t("retry")}</button>
          </div>
        ) : null}
        {!session.wsUrl && state === "idle" ? (
          <div className="tm-state">
            <button type="button" className="tm-tbtn" onClick={pane.reconnect}>{t("connect")}</button>
          </div>
        ) : null}
        {state === "disconnected" ? (
          <div className="tm-state over">
            <p className="tm-state-t">{stoppedText}</p>
            <button type="button" className="tm-tbtn" onClick={pane.reconnect}>{t("reconnect")}</button>
          </div>
        ) : null}
        {clipboardNotice ? (
          <p key={clipboardNotice.id} className="tm-notice" role="status" aria-live="polite">
            {clipboardNotice.action === "copy" ? t("copyBlocked") : t("pasteBlocked")}
          </p>
        ) : null}
      </div>

      {paneMenu ? (
        <TerminalPaneMenu
          paneMenu={paneMenu}
          isConnected={live}
          canReconnect={Boolean(targetNodeId || quickConnectParams)}
          closePaneMenu={() => setPaneMenu(null)}
          handleCopySelection={async () => {
            pane.report(await termRef.current?.copySelection(), "copy");
            setPaneMenu(null);
            termRef.current?.focus();
          }}
          handlePaste={async () => {
            pane.report(await termRef.current?.pasteFromClipboard(), "paste");
            setPaneMenu(null);
            termRef.current?.focus();
          }}
          handleSelectAll={closeMenuThen(() => termRef.current?.selectAll())}
          handleOpenSearch={closeMenuThen(() => setSearchOpen(true))}
          handleClearScrollback={closeMenuThen(() => {
            termRef.current?.clearScrollback();
            termRef.current?.focus();
          })}
          handleDisconnect={closeMenuThen(() => session.disconnect())}
          handleReconnectFromMenu={closeMenuThen(pane.reconnect)}
          handleChangeTarget={closeMenuThen(() => setPickerOpen(true))}
        />
      ) : null}

      <TerminalTargetPicker
        assets={assets}
        selectedTargetID={targetNodeId}
        connectedAgentIDs={session.connectedAgentIds}
        onSelect={handleTargetSelect}
        onAddress={requestAddress}
        open={pickerOpen}
        onOpenChange={setPickerOpen}
      />
    </section>
  );
}
