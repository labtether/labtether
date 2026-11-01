"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Plus, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useRef, type KeyboardEvent } from "react";
import type { TerminalSessionLibrary } from "./useTerminalSessionLibrary";
import type { TerminalWorkspace } from "./useTerminalWorkspace";
import { useTerminalWhen } from "./useTerminalWhen";

type TerminalTabStripProps = {
  ws: TerminalWorkspace;
  /** Each tab's label; null reads "New tab". */
  labels: Array<string | null>;
  /** Tabs with a shell that failed or dropped: a dot before the name. */
  troubled: Set<string>;
  lib: TerminalSessionLibrary;
  nameOf: (id: string) => string;
  onPlusOpenChange: (open: boolean) => void;
  onAddress: () => void;
  onSessions: () => void;
};

/**
 * The workspace tabs: one per stored tab, each with a close button, then "+" (a new tab, running sessions to reattach,
 * saved connections, Connect to an address…). Right-click (or Shift+F10) on a tab: rename, duplicate, close others.
 */
export default function TerminalTabStrip({ ws, labels, troubled, lib, nameOf, onPlusOpenChange, onAddress, onSessions }: TerminalTabStripProps) {
  const t = useTranslations("terminal.tabs");
  const when = useTerminalWhen();
  const { tabUI } = ws;
  const running = [...lib.detachedSessions, ...lib.activeSessions].slice(0, 6);
  const saved = lib.savedBookmarks.slice(0, 6);

  const tabKeys = (event: KeyboardEvent<HTMLButtonElement>, tabId: string) => {
    if (event.key === "F2") {
      const tab = ws.tabs.find((entry) => entry.id === tabId);
      if (tab) {
        event.preventDefault();
        tabUI.beginRenameTab(tab);
      }
    } else if (event.key === "ContextMenu" || (event.shiftKey && event.key === "F10")) {
      event.preventDefault();
      const rect = event.currentTarget.getBoundingClientRect();
      tabUI.openTabMenuAt(tabId, rect.left, rect.bottom + 4);
    }
  };

  return (
    <nav className="tm-tabs" aria-label={t("label")}>
      {ws.tabs.map((tab, index) => {
        const label = labels[index] ?? t("newTab");
        const on = tab.id === ws.activeTabId;
        const editing = tabUI.editingTabId === tab.id;
        return (
          <div key={tab.id} className={`tm-tab${on ? " on" : ""}`} data-tab-id={tab.id} onContextMenu={(event) => tabUI.openTabMenu(event, tab.id)}>
            {editing ? (
              <input
                ref={tabUI.tabRenameInputRef}
                className="tm-tab-in"
                aria-label={t("renameLabel")}
                value={tabUI.editingTabName}
                maxLength={48}
                onChange={(event) => tabUI.setEditingTabName(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    void tabUI.commitRenameTab();
                  } else if (event.key === "Escape") {
                    event.preventDefault();
                    tabUI.cancelRenameTab();
                  }
                }}
                onBlur={tabUI.handleRenameInputBlur}
              />
            ) : (
              <button
                type="button"
                className="tm-tb"
                aria-current={on ? "true" : undefined}
                title={label}
                onClick={() => ws.setActiveTab(tab.id)}
                onKeyDown={(event) => tabKeys(event, tab.id)}
              >
                {troubled.has(tab.id) ? <i className="kt-dot sm bad" aria-hidden="true" /> : null}
                <span>{label}</span>
              </button>
            )}
            {editing ? null : (
              <button type="button" className="tm-x" aria-label={t("close", { name: label })} onClick={() => ws.closeTab(tab.id)}>
                <X aria-hidden="true" />
              </button>
            )}
          </div>
        );
      })}
      <Menu.Root onOpenChange={onPlusOpenChange}>
        <Menu.Trigger className="tm-icon tm-plus" aria-label={t("newTab")} title={t("newTab")}>
          <Plus aria-hidden="true" />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu tm-menu" align="start" sideOffset={6}>
            <Menu.Item className="lt-menu-item" onSelect={tabUI.handleCreateTab}>{t("newTab")}</Menu.Item>
            {running.length > 0 ? (
              <>
                <Menu.Separator className="lt-menu-sep" />
                <Menu.Label className="tm-menu-h">{t("running")}</Menu.Label>
                {running.map((s) => (
                  <Menu.Item key={s.id} className="lt-menu-item" onSelect={() => void lib.reattach(s, true)}>
                    {t("reattach", { name: s.title || nameOf(s.target) || s.target })}
                    <span className="lt-menu-hint">{s.status === "attached" ? t("attached") : when(s.last_detached_at || s.updated_at)}</span>
                  </Menu.Item>
                ))}
              </>
            ) : null}
            {saved.length > 0 ? (
              <>
                <Menu.Separator className="lt-menu-sep" />
                <Menu.Label className="tm-menu-h">{t("saved")}</Menu.Label>
                {saved.map((b) => (
                  <Menu.Item key={b.id} className="lt-menu-item" onSelect={() => void lib.connectBookmark(b, true)}>
                    {b.title}
                  </Menu.Item>
                ))}
              </>
            ) : null}
            <Menu.Separator className="lt-menu-sep" />
            <Menu.Item className="lt-menu-item" onSelect={onAddress}>{t("address")}</Menu.Item>
            <Menu.Item className="lt-menu-item" onSelect={onSessions}>{t("sessions")}</Menu.Item>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </nav>
  );
}

/** The tab's right-click menu, placed where the click was. */
export function TerminalTabMenu({ ws }: { ws: TerminalWorkspace }) {
  const t = useTranslations("terminal.tabs");
  const { tabUI } = ws;
  const target = tabUI.tabMenuTarget;
  const menuRef = useRef<HTMLDivElement | null>(null);
  const openFor = tabUI.tabMenu?.tabId;
  useEffect(() => {
    if (openFor) menuRef.current?.querySelector<HTMLButtonElement>("button")?.focus();
  }, [openFor]);
  if (!tabUI.tabMenu || !target) return null;
  const others = ws.tabs.length > 1;
  const item = (label: string, onClick: () => void, disabled = false) => (
    <button type="button" role="menuitem" className="lt-menu-item tm-mi" disabled={disabled} onClick={onClick}>{label}</button>
  );
  return (
    <div
      className="tm-ctx-back"
      onClick={tabUI.closeTabMenu}
      onContextMenu={(event) => {
        event.preventDefault();
        tabUI.closeTabMenu();
      }}
    >
      <div
        role="menu"
        aria-label={t("menu")}
        className="lt-menu tm-ctx"
        style={{ left: tabUI.tabMenu.x, top: tabUI.tabMenu.y }}
        ref={menuRef}
        onClick={(event) => event.stopPropagation()}
      >
        {item(t("rename"), () => tabUI.beginRenameTab(target))}
        {item(t("duplicate"), () => {
          tabUI.closeTabMenu();
          void tabUI.handleDuplicateTab(target);
        })}
        {item(t("newTab"), () => {
          tabUI.closeTabMenu();
          tabUI.handleCreateTab();
        })}
        <div className="lt-menu-sep" role="separator" />
        {item(t("closeTab"), () => {
          tabUI.closeTabMenu();
          ws.closeTab(target.id);
        })}
        {item(t("closeOthers"), () => {
          tabUI.closeTabMenu();
          void tabUI.handleCloseOtherTabs(target.id);
        }, !others)}
      </div>
    </div>
  );
}
