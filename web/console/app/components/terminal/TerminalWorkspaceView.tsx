"use client";

import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "../../../i18n/navigation";
import type { TerminalPreferences } from "../../hooks/useTerminalPreferences";
import type { TerminalSnippet } from "../../hooks/useTerminalSnippets";
import type { TerminalFontDef } from "../../terminal/fonts";
import type { TerminalThemeDef } from "../../terminal/themes";
import type { XTerminalHandle } from "../XTerminal";
import KeyboardToolbar from "./KeyboardToolbar";
import TerminalMoreMenu from "./TerminalMoreMenu";
import TerminalPane, { type PaneControls, type PaneStatus } from "./TerminalPane";
import TerminalTabStrip, { TerminalTabMenu } from "./TerminalTabStrip";
import { tabLabels } from "./terminalTargets";
import type { TerminalSessionLibrary } from "./useTerminalSessionLibrary";
import { paneKey, type TerminalWorkspace } from "./useTerminalWorkspace";
import WorkspaceGrid from "./WorkspaceGrid";

type TerminalWorkspaceViewProps = {
  ws: TerminalWorkspace;
  prefs: TerminalPreferences;
  themeDef: TerminalThemeDef;
  fontDef: TerminalFontDef;
  snippets: TerminalSnippet[];
  lib: TerminalSessionLibrary;
  nameOf: (id: string) => string;
  fullscreen: boolean;
  setFullscreen: (on: boolean) => void;
  onSettings: () => void;
  onAllSnippets: () => void;
  onSessions: () => void;
  onPlusOpenChange: (open: boolean) => void;
};

function useNarrow() {
  const [narrow, setNarrow] = useState(false);
  useEffect(() => {
    const query = window.matchMedia("(max-width: 640px)");
    const update = () => setNarrow(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);
  return narrow;
}

type SlotProps = {
  ws: TerminalWorkspace;
  tabId: string;
  index: number;
  targetNodeId: string;
  isActive: boolean;
  split: boolean;
  prefs: TerminalPreferences;
  themeDef: TerminalThemeDef;
  fontDef: TerminalFontDef;
};

/** One pane of a tab, with callbacks that stay the same between renders. */
function PaneSlot({ ws, tabId, index, targetNodeId, isActive, split, prefs, themeDef, fontDef }: SlotProps) {
  const key = paneKey(tabId, index);
  const { setPaneRef, setControls, reportStatus, setPaneTarget, setActiveTab, setFocusedPaneIndex, setAddressRequest } = ws;
  const onTerminalRef = useCallback((handle: XTerminalHandle | null) => setPaneRef(tabId, index, handle), [setPaneRef, tabId, index]);
  const onControls = useCallback((controls: PaneControls | null) => setControls(key, controls), [setControls, key]);
  const onStatusChange = useCallback((status: PaneStatus) => reportStatus(key, status), [reportStatus, key]);
  const onTargetChange = useCallback((id: string) => setPaneTarget(tabId, index, id), [setPaneTarget, tabId, index]);
  const onRequestAddress = useCallback((initial: string) => setAddressRequest({ paneKey: key, initial }), [setAddressRequest, key]);
  const onFocus = useCallback(() => {
    if (!isActive) setActiveTab(tabId);
    setFocusedPaneIndex(index);
  }, [isActive, setActiveTab, setFocusedPaneIndex, tabId, index]);
  return (
    <TerminalPane
      paneIndex={index}
      targetNodeId={targetNodeId}
      onTargetChange={onTargetChange}
      isTabActive={isActive}
      isFocused={isActive && ws.focusedPaneIndex === index}
      onFocus={onFocus}
      broadcastActive={ws.broadcastActive && isActive}
      onBroadcastData={ws.broadcast}
      onTerminalRef={onTerminalRef}
      prefs={prefs}
      themeDef={themeDef}
      fontDef={fontDef}
      split={split}
      quickConnectParams={ws.quickParams[key]}
      onRequestAddress={onRequestAddress}
      onStatusChange={onStatusChange}
      onControls={onControls}
    />
  );
}

/** The open workspace: tabs, the focused pane's status and the ⋯ menu, then the panes of every tab (one shown). */
export default function TerminalWorkspaceView({
  ws, prefs, themeDef, fontDef, snippets, lib, nameOf, fullscreen, setFullscreen, onSettings, onAllSnippets, onSessions, onPlusOpenChange,
}: TerminalWorkspaceViewProps) {
  const t = useTranslations("terminal");
  const narrow = useNarrow();
  const labels = useMemo(() => tabLabels(ws.tabs, nameOf), [ws.tabs, nameOf]);
  const split = ws.paneCount > 1;
  const status = ws.focusedStatus;
  const controls = () => ws.focusedControls();
  const focusedPane = ws.panes[ws.focusedPaneIndex];
  const focusedTarget = focusedPane?.targetNodeId ?? "";
  const quick = ws.quickParams[ws.focusedKey];
  const crumbName = quick ? `${quick.username}@${quick.host}` : focusedTarget ? nameOf(focusedTarget) || focusedTarget : "";
  const live = status?.state === "connected";
  const troubled = useMemo(() => new Set(
    Object.entries(ws.paneStatus)
      .filter(([, pane]) => pane.state === "error" || pane.state === "disconnected")
      .map(([key]) => key.slice(0, key.lastIndexOf(":"))),
  ), [ws.paneStatus]);

  const statusText = !status || split ? "" : status.state === "connected"
    ? status.via === "agent" ? t("pane.connected") : t("pane.connectedVia", { via: t(`pane.via_${status.via}`) })
    : status.state === "connecting" ? t("pane.connecting")
      : status.state === "error" ? t("pane.failed")
        : status.state === "disconnected" ? t("pane.disconnected") : "";

  return (
    <div ref={ws.tabUI.tabMenuHostRef} className={`tm-root${fullscreen ? " full" : ""}`}>
      <h1 className="sr-only">{t("title")}</h1>
      <nav className="tm-crumb" aria-label={t("crumb")}>
        <span>{t("title")}</span>
        {crumbName ? (
          <>
            <span aria-hidden="true">/</span>
            {focusedTarget && !quick ? <Link href={`/nodes/${encodeURIComponent(focusedTarget)}`}>{crumbName}</Link> : <span>{crumbName}</span>}
          </>
        ) : null}
      </nav>
      <div className="tm-top">
        <TerminalTabStrip
          ws={ws}
          labels={labels}
          troubled={troubled}
          lib={lib}
          nameOf={nameOf}
          onPlusOpenChange={onPlusOpenChange}
          onAddress={() => ws.setAddressRequest({ paneKey: null, initial: "" })}
          onSessions={onSessions}
        />
        <div className="tm-bar">
          {statusText ? <span className={`tm-st${status?.state === "error" ? " bad" : ""}`}>{statusText}</span> : null}
          {!split && live ? (
            <button type="button" className="tm-qbtn" title={t("pane.disconnect")} onClick={() => controls()?.disconnect()}>
              {t("pane.disconnect")}
            </button>
          ) : null}
          {ws.tabs.length > 0 ? (
            <TerminalMoreMenu
              hasShell={Boolean(status) && status?.state !== "empty"}
              layout={ws.layout}
              paneCount={ws.paneCount}
              snippets={snippets}
              broadcastActive={ws.broadcastActive}
              fullscreen={fullscreen}
              onCopyAll={() => controls()?.copyAll()}
              onClear={() => controls()?.clear()}
              onFind={() => controls()?.openFind()}
              onChangeDevice={() => controls()?.changeTarget()}
              onLayout={ws.tabUI.handleLayoutChange}
              onSnippet={ws.insertSnippet}
              onAllSnippets={onAllSnippets}
              onSettings={onSettings}
              onBroadcast={ws.setBroadcastActive}
              onFullscreen={setFullscreen}
            />
          ) : null}
        </div>
      </div>

      {ws.workspaceError ? <p className="tm-err" role="alert">{t(`errors.${ws.workspaceError}`)}</p> : null}

      {ws.tabs.length === 0 ? (
        <div className="tm-pane slot tm-noload">
          <p className="tm-none">{t("notLoaded")}</p>
        </div>
      ) : (
        <div className={`tm-ws${split ? " split" : ""}${ws.broadcastActive ? " all" : ""}`}>
          {ws.tabPanels.map((panel) => {
            const isActive = panel.tabId === ws.activeTabId;
            return (
              <div key={panel.tabId} className={`tm-panel${isActive ? "" : " off"}`} aria-hidden={!isActive}>
                <WorkspaceGrid
                  layout={panel.layout}
                  paneCount={panel.paneCount}
                  panelSizes={panel.panelSizes}
                  onPanelResize={(sizes) => ws.resizePanels(panel.tabId, sizes)}
                  stacked={narrow}
                >
                  {panel.panes.slice(0, panel.paneCount).map((pane, index) => (
                    <PaneSlot
                      key={`${panel.tabId}-pane-${index}`}
                      ws={ws}
                      tabId={panel.tabId}
                      index={index}
                      targetNodeId={pane.targetNodeId}
                      isActive={isActive}
                      split={panel.paneCount > 1}
                      prefs={prefs}
                      themeDef={themeDef}
                      fontDef={fontDef}
                    />
                  ))}
                </WorkspaceGrid>
              </div>
            );
          })}
        </div>
      )}

      <KeyboardToolbar termRef={ws.focusedTermRef} keys={prefs.toolbar_keys ?? undefined} />
      <TerminalTabMenu ws={ws} />
    </div>
  );
}
