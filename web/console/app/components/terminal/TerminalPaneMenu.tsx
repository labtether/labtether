"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef } from "react";

export type PaneContextMenuState = {
  x: number;
  y: number;
  hasSelection: boolean;
};

type TerminalPaneMenuProps = {
  paneMenu: PaneContextMenuState;
  isConnected: boolean;
  canReconnect: boolean;
  closePaneMenu: () => void;
  handleCopySelection: () => Promise<void>;
  handlePaste: () => Promise<void>;
  handleSelectAll: () => void;
  handleOpenSearch: () => void;
  handleClearScrollback: () => void;
  handleDisconnect: () => void;
  handleReconnectFromMenu: () => void;
  /** Opens the device picker for this pane. */
  handleChangeTarget?: () => void;
};

/** The pane's right-click menu: clipboard, Find, Clear, and Disconnect or Reconnect. */
export function TerminalPaneMenu({ paneMenu, isConnected, canReconnect, closePaneMenu,
  handleCopySelection, handlePaste, handleSelectAll, handleOpenSearch,
  handleClearScrollback, handleDisconnect, handleReconnectFromMenu, handleChangeTarget,
}: TerminalPaneMenuProps) {
  const t = useTranslations("terminal.pane");
  const menuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    menuRef.current?.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus();
  }, []);

  const item = (label: string, onClick: () => void, disabled = false, danger = false) => (
    <button
      type="button"
      role="menuitem"
      className={`lt-menu-item tm-mi${danger ? " tm-danger" : ""}`}
      disabled={disabled}
      onClick={onClick}
    >
      {label}
    </button>
  );

  return (
    <div
      className="tm-ctx-back"
      onClick={closePaneMenu}
      onContextMenu={(event) => {
        event.preventDefault();
        closePaneMenu();
      }}
    >
      <div
        ref={menuRef}
        role="menu"
        aria-label={t("menu")}
        className="lt-menu tm-ctx"
        style={{ left: paneMenu.x, top: paneMenu.y }}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
          event.preventDefault();
          const items = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)") ?? []);
          const at = items.indexOf(document.activeElement as HTMLButtonElement);
          items[(at + (event.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus();
        }}
      >
        {item(t("copy"), () => void handleCopySelection(), !paneMenu.hasSelection)}
        {item(t("paste"), () => void handlePaste())}
        {item(t("selectAll"), handleSelectAll)}
        <div className="lt-menu-sep" role="separator" />
        {item(t("find"), handleOpenSearch)}
        {item(t("clear"), handleClearScrollback)}
        {handleChangeTarget ? item(t("changeDevice"), handleChangeTarget) : null}
        <div className="lt-menu-sep" role="separator" />
        {isConnected
          ? item(t("disconnect"), handleDisconnect, false, true)
          : item(t("reconnect"), handleReconnectFromMenu, !canReconnect)}
      </div>
    </div>
  );
}
