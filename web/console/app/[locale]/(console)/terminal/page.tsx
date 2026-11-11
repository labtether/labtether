"use client";

import dynamic from "next/dynamic";
import { useTranslations } from "next-intl";
import { useCallback, useMemo, useState } from "react";
import { useAuth } from "../../../contexts/AuthContext";
import { useFastStatus } from "../../../contexts/StatusContext";
import { useTheme } from "../../../contexts/ThemeContext";
import { useConnectedAgents } from "../../../hooks/useConnectedAgents";
import { useTerminalPreferences } from "../../../hooks/useTerminalPreferences";
import { useTerminalSnippets } from "../../../hooks/useTerminalSnippets";
import { canAccessHostData } from "../../../lib/roles";
import { isHiddenAsset } from "../../../console/taxonomy";
import { getFontById } from "../../../terminal/fonts";
import { OperatorAccessRequired } from "../../../components/OperatorAccessRequired";
import BookmarkDialog from "../../../components/terminal/BookmarkDialog";
import QuickConnectDialog from "../../../components/terminal/QuickConnectDialog";
import SessionsPanel from "../../../components/terminal/SessionsPanel";
import SettingsPanel from "../../../components/terminal/SettingsPanel";
import SnippetPicker from "../../../components/terminal/SnippetPicker";
import TerminalLanding from "../../../components/terminal/TerminalLanding";
import TerminalWorkspaceView from "../../../components/terminal/TerminalWorkspaceView";
import { formatAddress } from "../../../components/terminal/terminalTargets";
import { resolveTerminalTheme } from "../../../components/terminal/terminalLook";
import type { Bookmark } from "../../../components/terminal/terminalSessionTypes";
import type { QuickConnectParams } from "../../../hooks/useSession";
import { useTerminalPageKeys } from "../../../components/terminal/useTerminalPageKeys";
import { useTerminalSessionLibrary } from "../../../components/terminal/useTerminalSessionLibrary";
import { useTerminalWorkspace } from "../../../components/terminal/useTerminalWorkspace";
import { useHostKeyGate } from "../../../components/terminal/useHostKeyGate";
import { useQuickConnectFlow } from "../../../components/terminal/useQuickConnectFlow";

const ScrollbackViewer = dynamic(() => import("../../../components/terminal/ScrollbackViewer"), { ssr: false });

export default function TerminalWorkspacePage() {
  const { user } = useAuth();
  const tNav = useTranslations("nav");
  if (!canAccessHostData(user?.role)) {
    return <OperatorAccessRequired title={tNav("terminal")} feature="terminal" />;
  }
  return <TerminalWorkspacePageContent />;
}

/**
 * Terminal: the start page while nothing is open, else the workspace of tabs and panes. The dialogs (address, saved
 * connection, sessions, settings, snippets, ended session output) belong to the page so every part can open them.
 */
function TerminalWorkspacePageContent() {
  const t = useTranslations("terminal");
  const ws = useTerminalWorkspace();
  const { prefs, updatePrefs } = useTerminalPreferences();
  const { snippets } = useTerminalSnippets();
  const { theme: appTheme } = useTheme();
  const { connectedAgentIds } = useConnectedAgents();
  const status = useFastStatus();
  const [fullscreen, setFullscreen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(false);
  const [plusOpen, setPlusOpen] = useState(false);
  const [archived, setArchived] = useState<{ id: string; title: string } | null>(null);

  const assets = useMemo(() => (status?.assets ?? []).filter((asset) => !isHiddenAsset(asset)), [status?.assets]);
  const names = useMemo(() => new Map(assets.map((asset) => [asset.id, asset.name || asset.id])), [assets]);
  const nameOf = useCallback((id: string) => names.get(id) ?? "", [names]);
  const devices = useMemo(() => assets.map((asset) => ({ id: asset.id, name: asset.name || asset.id })).sort((a, b) => a.name.localeCompare(b.name)), [assets]);

  const hostKeys = useHostKeyGate();
  const { openTarget, setAddressRequest } = ws;
  const { handleCreateTab } = ws.tabUI;
  const onSessionSelect = useCallback((_id: string, _persistentId: string, _ticket: string, newTab: boolean, target?: string) => {
    setSessionsOpen(false);
    if (target) void openTarget(target, newTab);
    else if (newTab) handleCreateTab();
  }, [handleCreateTab, openTarget]);
  const onBookmarkConnect = useCallback((_id: string, newTab: boolean, bookmark?: Bookmark) => {
    setSessionsOpen(false);
    if (bookmark?.asset_id) void openTarget(bookmark.asset_id, newTab);
    // A bookmark with a saved login opens its own saved session (host:port); one without asks for the login.
    else if (bookmark?.host && bookmark.credential_profile_id) void openTarget(`${bookmark.host}:${bookmark.port ?? 22}`, newTab);
    else if (bookmark?.host) {
      setAddressRequest({ paneKey: null, initial: formatAddress({ user: bookmark.username ?? "", host: bookmark.host, port: bookmark.port ?? 22 }) });
    }
  }, [openTarget, setAddressRequest]);
  const closeAddressRequest = useCallback(() => setAddressRequest(null), [setAddressRequest]);
  const { connectAddress } = ws;
  const connectQuick = useCallback((params: QuickConnectParams) => void connectAddress(params), [connectAddress]);
  const quick = useQuickConnectFlow({
    requested: ws.addressRequest !== null,
    closeRequest: closeAddressRequest,
    connect: connectQuick,
    gate: hostKeys.gate,
  });
  const lib = useTerminalSessionLibrary({
    isOpen: ws.fresh || sessionsOpen || plusOpen,
    onSessionSelect,
    onBookmarkConnect,
    onArchivedView: (id, title) => setArchived({ id, title }),
    hostKeyGate: hostKeys.gate,
  });

  const toggleSnippets = useCallback(() => setSnippetsOpen((open) => !open), []);
  useTerminalPageKeys({ ws, ready: !ws.loading, fullscreen, setFullscreen, toggleSnippets });

  const themeDef = useMemo(() => resolveTerminalTheme(prefs.theme, appTheme), [prefs.theme, appTheme]);
  const fontDef = useMemo(() => getFontById(prefs.font_family), [prefs.font_family]);

  let body;
  if (ws.loading) {
    body = (
      <>
        <h1 className="sr-only">{t("title")}</h1>
        <p className="tm-loading kt-loading">{t("loading")}</p>
      </>
    );
  } else if (ws.fresh) {
    body = (
      <TerminalLanding
        assets={assets}
        connectedAgentIds={connectedAgentIds}
        lib={lib}
        nameOf={nameOf}
        onPick={(id) => void openTarget(id)}
        onAddress={(initial) => setAddressRequest({ paneKey: null, initial })}
        onSaveConnection={() => lib.setBookmarkDialogOpen(true)}
        onSettings={() => setSettingsOpen(true)}
      />
    );
  } else {
    body = (
      <TerminalWorkspaceView
        ws={ws}
        prefs={prefs}
        themeDef={themeDef}
        fontDef={fontDef}
        snippets={snippets}
        lib={lib}
        nameOf={nameOf}
        fullscreen={fullscreen}
        setFullscreen={setFullscreen}
        onSettings={() => setSettingsOpen(true)}
        onAllSnippets={() => setSnippetsOpen(true)}
        onSessions={() => setSessionsOpen(true)}
        onPlusOpenChange={setPlusOpen}
      />
    );
  }

  return (
    <>
      {body}
      <QuickConnectDialog
        open={quick.open}
        initialAddress={ws.addressRequest?.initial ?? ""}
        draft={quick.draft}
        onClose={quick.onClose}
        onConnect={quick.onConnect}
      />
      {hostKeys.dialog}
      <SessionsPanel
        open={sessionsOpen}
        onClose={() => setSessionsOpen(false)}
        lib={lib}
        nameOf={nameOf}
        onAddConnection={() => lib.setBookmarkDialogOpen(true)}
      />
      <BookmarkDialog
        isOpen={lib.bookmarkDialogOpen}
        onClose={() => lib.setBookmarkDialogOpen(false)}
        onSave={(data) => void lib.handleBookmarkSave(data)}
        devices={devices}
      />
      <SettingsPanel open={settingsOpen} onClose={() => setSettingsOpen(false)} prefs={prefs} onUpdatePrefs={updatePrefs} />
      <SnippetPicker snippets={snippets} open={snippetsOpen} onClose={() => setSnippetsOpen(false)} onSelect={ws.insertSnippet} />
      {archived ? <ScrollbackViewer persistentSessionId={archived.id} title={archived.title} onClose={() => setArchived(null)} /> : null}
    </>
  );
}
