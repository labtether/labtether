"use client";

import { useCallback, useState } from "react";
import { useRemoteViewTabsState } from "./useRemoteViewTabsState";
import RemoteViewTabBar from "./RemoteViewTabBar";
import NewTabPage from "./NewTabPage";
import RemoteViewSession from "./RemoteViewSession";
import { defaultPort } from "./types";
import type { RemoteViewProtocol, RemoteViewConnectionState } from "./types";
import {
  getBookmarkCredentials,
  createBookmark,
  type RemoteBookmark,
} from "./remoteBookmarksClient";

// ── Page ──

export default function RemoteViewPage() {
  const tabs = useRemoteViewTabsState();
  const activeTab = tabs.activeTab;

  // Credentials to pass to the active session (keyed by tabId so they reset
  // when the tab changes or a new connection is started).
  const [sessionCredentials, setSessionCredentials] = useState<
    Record<string, { username?: string; password?: string }>
  >({});

  const isNewTab = activeTab?.type === "new";
  const isSessionTab =
    activeTab?.type === "device" ||
    activeTab?.type === "bookmark" ||
    activeTab?.type === "adhoc";

  const handleConnectDevice = useCallback(
    (assetId: string, name: string, protocol: RemoteViewProtocol) => {
      // Device connections use agent-based auth — no credential prompt needed.
      tabs.updateTab(activeTab.id, {
        type: "device",
        label: name,
        protocol,
        target: { host: "", port: defaultPort(protocol), assetId },
        connectionState: "connecting",
      });
    },
    [tabs, activeTab],
  );

  const handleConnectBookmark = useCallback(
    (bookmark: RemoteBookmark) => {
      const tabId = activeTab.id;

      const startConnection = (creds: { username?: string; password?: string }) => {
        setSessionCredentials((prev) => ({ ...prev, [tabId]: creds }));
        tabs.updateTab(tabId, {
          type: "bookmark",
          label: bookmark.label,
          protocol: bookmark.protocol,
          target: {
            host: bookmark.host,
            port: bookmark.port,
            bookmarkId: bookmark.id,
          },
          connectionState: "connecting",
        });
      };

      if (bookmark.has_credentials) {
        // Fetch stored credentials then connect
        getBookmarkCredentials(bookmark.id)
          .then((creds) => startConnection(creds))
          .catch(() => startConnection({}));
      } else {
        // No stored credentials — connect without
        startConnection({});
      }
    },
    [tabs, activeTab],
  );

  const handleConnectAdhoc = useCallback(
    (params: {
      host: string;
      port: number;
      protocol: RemoteViewProtocol;
      username?: string;
      password?: string;
      saveBookmark?: { label: string };
    }) => {
      const { host, port, protocol, username, password, saveBookmark } = params;
      const tabId = activeTab.id;

      const creds = {
        username: username || undefined,
        password: password || undefined,
      };
      setSessionCredentials((prev) => ({ ...prev, [tabId]: creds }));
      tabs.updateTab(tabId, {
        type: "adhoc",
        label: saveBookmark?.label || host,
        protocol,
        target: { host, port },
        connectionState: "connecting",
      });

      // Save as bookmark if requested (fire-and-forget)
      if (saveBookmark) {
        createBookmark({
          label: saveBookmark.label,
          protocol,
          host,
          port,
          username,
          password,
        }).catch((err) => console.error("Failed to save bookmark:", err));
      }
    },
    [tabs, activeTab],
  );

  const handleConnectionStateChange = useCallback(
    (state: RemoteViewConnectionState) => {
      tabs.setConnectionState(activeTab.id, state);
    },
    [tabs, activeTab],
  );

  return (
    <div className="flex flex-col h-full">
      <RemoteViewTabBar
        tabs={tabs.tabs}
        activeTabId={tabs.activeTabId}
        onAddTab={() => tabs.addTab()}
        onRemoveTab={tabs.removeTab}
        onSetActiveTab={tabs.setActiveTab}
        connectionState={activeTab?.connectionState}
        latencyMs={null}
      />

      {isNewTab && (
        <NewTabPage
          onConnectDevice={handleConnectDevice}
          onConnectBookmark={handleConnectBookmark}
          onConnectAdhoc={handleConnectAdhoc}
        />
      )}

      {isSessionTab && activeTab && (
        <RemoteViewSession
          key={activeTab.id}
          tab={activeTab}
          initialCredentials={sessionCredentials[activeTab.id]}
          onConnectionStateChange={handleConnectionStateChange}
        />
      )}

    </div>
  );
}
