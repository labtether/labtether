"use client";

import { Plus, X } from "lucide-react";
import { useTranslations } from "next-intl";
import type { FileTab } from "./useFileTabsState";

interface FileTabBarProps {
  tabs: FileTab[];
  activeTabId: string;
  onAddTab: () => void;
  onRemoveTab: (tabId: string) => void;
  onSetActiveTab: (tabId: string) => void;
  /** Tabs whose source can't be browsed now (an agent that is not connected) show a red dot. */
  unavailable?: ReadonlySet<string>;
}

/** The Files tabs: one per device or connection, each with its own close button, and + for a new tab. */
export function FileTabBar({ tabs, activeTabId, onAddTab, onRemoveTab, onSetActiveTab, unavailable }: FileTabBarProps) {
  const t = useTranslations("files.tabs");
  return (
    <div className="fx-tabs">
      <div role="tablist" aria-label={t("label")} className="fx-tablist">
        {tabs.map((tab) => {
          const on = tab.id === activeTabId;
          const name = tab.type === "new" ? t("newTab") : tab.label;
          return (
            <div key={tab.id} className={`fx-tab${on ? " on" : ""}`}>
              <button type="button" role="tab" aria-selected={on} className="fx-tab-b" title={name} onClick={() => onSetActiveTab(tab.id)}>
                {unavailable?.has(tab.id) ? <span className="fx-dot bad" aria-hidden="true" /> : null}
                <span className="fx-tab-n">{name}</span>
              </button>
              <button type="button" className="fx-tab-x" aria-label={t("close", { name })} title={t("close", { name })} onClick={() => onRemoveTab(tab.id)}>
                <X aria-hidden="true" width={12} height={12} strokeWidth={2} />
              </button>
            </div>
          );
        })}
      </div>
      <button type="button" className="fx-tab-plus" aria-label={t("new")} title={t("new")} onClick={onAddTab}>
        <Plus aria-hidden="true" width={15} height={15} strokeWidth={2} />
      </button>
    </div>
  );
}
