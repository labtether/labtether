"use client";

import { useCallback, useState } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import { useRemoteViewTabsState } from "./useRemoteViewTabsState";
import RemoteViewTabBar from "./RemoteViewTabBar";
import NewTabPage from "./NewTabPage";
import RemoteViewSession from "./RemoteViewSession";
import { defaultPort } from "./types";
import type { RemoteViewProtocol, RemoteViewConnectionState } from "./types";
import type { AdhocConnectParams } from "./quickConnectModel";
import { useAuth } from "../../../contexts/AuthContext";
import { canAccessHostData } from "../../../lib/roles";
import { OperatorAccessRequired } from "../../../components/OperatorAccessRequired";
import {
  getBookmarkCredentials,
  createBookmark,
  type RemoteBookmark,
} from "./remoteBookmarksClient";

// ── Page ──

export default function RemoteViewPage() {
  const { user } = useAuth();
  const tNav = useTranslations("nav");

  if (!canAccessHostData(user?.role)) {
    return <OperatorAccessRequired title={tNav("remoteView")} feature="remoteView" />;
  }

  return <RemoteViewPageContent />;
}

function RemoteViewPageContent() {
  const t = useTranslations("desktop.page");
  const tNav = useTranslations("nav");
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
            allowInsecureVNC: bookmark.allow_insecure_vnc,
            spiceSecurityMode: bookmark.spice_security_mode,
            spiceCAPEM: bookmark.spice_ca_pem,
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
    (params: AdhocConnectParams) => {
      const {
        host,
        port,
        protocol,
        username,
        password,
        allowInsecureVNC,
        ignoreCertificate,
        allowLegacySecurity,
        certificateFingerprints,
        spiceSecurityMode,
        spiceCAPEM,
        saveBookmark,
      } = params;
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
        target: {
          host,
          port,
          allowInsecureVNC,
          ignoreCertificate,
          allowLegacySecurity,
          certificateFingerprints,
          spiceSecurityMode,
          spiceCAPEM,
        },
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
          allow_insecure_vnc: allowInsecureVNC,
          spice_security_mode: spiceSecurityMode,
          spice_ca_pem: spiceCAPEM,
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

  const tabBar = (
    <RemoteViewTabBar
      tabs={tabs.tabs}
      activeTabId={tabs.activeTabId}
      onAddTab={() => tabs.addTab()}
      onRemoveTab={tabs.removeTab}
      onSetActiveTab={tabs.setActiveTab}
    />
  );

  if (isSessionTab && activeTab) {
    const deviceId = activeTab.type === "device" ? activeTab.target?.assetId : undefined;
    return (
      <div className="rv-page">
        <nav className="rv-crumb" aria-label={t("breadcrumb")}>
          <span>{tNav("remoteView")}</span>
          <span aria-hidden="true">/</span>
          {deviceId ? <Link href={`/nodes/${encodeURIComponent(deviceId)}`}>{activeTab.label}</Link> : <span>{activeTab.label}</span>}
        </nav>
        <h1 className="sr-only">{`${tNav("remoteView")} · ${activeTab.label}`}</h1>
        <RemoteViewSession
          key={activeTab.id}
          tab={activeTab}
          tabBar={tabBar}
          initialCredentials={sessionCredentials[activeTab.id]}
          onConnectionStateChange={handleConnectionStateChange}
        />
      </div>
    );
  }

  return (
    <div className="rv-page">
      {isNewTab ? (
        <NewTabPage
          tabBar={tabs.tabs.length > 1 ? tabBar : undefined}
          onConnectDevice={handleConnectDevice}
          onConnectBookmark={handleConnectBookmark}
          onConnectAdhoc={handleConnectAdhoc}
        />
      ) : null}
    </div>
  );
}
