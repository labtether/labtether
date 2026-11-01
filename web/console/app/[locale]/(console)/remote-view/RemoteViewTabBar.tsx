"use client";

import { Plus, X } from "lucide-react";
import { useTranslations } from "next-intl";
import type { RemoteViewConnectionState, RemoteViewTab } from "./types";

interface RemoteViewTabBarProps {
  tabs: RemoteViewTab[];
  activeTabId: string;
  onAddTab: () => void;
  onRemoveTab: (tabId: string) => void;
  onSetActiveTab: (tabId: string) => void;
}

/** A tab's dot only when its screen is not simply connected: starting, failed or ended. */
function tone(state: RemoteViewConnectionState): string {
  if (state === "connecting" || state === "authenticating") return "busy";
  if (state === "error") return "bad";
  if (state === "disconnected") return "off";
  return "";
}

/** Remote View's tabs, each with its close button, and "+" for a new tab. */
export default function RemoteViewTabBar({ tabs, activeTabId, onAddTab, onRemoveTab, onSetActiveTab }: RemoteViewTabBarProps) {
  const t = useTranslations("desktop.tabs");
  return (
    <div className="rv-tabs">
      <div role="tablist" aria-label={t("label")} className="rv-tablist">
        {tabs.map((tab) => {
          const active = tab.id === activeTabId;
          const label = tab.type === "new" ? t("newTab") : tab.label;
          const dot = tone(tab.connectionState);
          return (
            <div key={tab.id} className={`rv-tab${active ? " on" : ""}`}>
              <button type="button" role="tab" aria-selected={active} className="rv-tab-b" onClick={() => onSetActiveTab(tab.id)}>
                {dot ? <i className={`rv-dot ${dot}`} aria-hidden="true" /> : null}
                <span>{label}</span>
              </button>
              <button type="button" className="rv-tab-x" aria-label={t("close", { name: label })} onClick={() => onRemoveTab(tab.id)}>
                <X aria-hidden="true" />
              </button>
            </div>
          );
        })}
      </div>
      <button type="button" className="rv-ib sm rv-tab-add" aria-label={t("newTab")} title={t("newTab")} onClick={onAddTab}>
        <Plus aria-hidden="true" />
      </button>
    </div>
  );
}
