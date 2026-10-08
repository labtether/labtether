"use client";

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
};

export function TerminalPaneMenu({ paneMenu, isConnected, canReconnect, closePaneMenu,
  handleCopySelection, handlePaste, handleSelectAll, handleOpenSearch,
  handleClearScrollback, handleDisconnect, handleReconnectFromMenu,
}: TerminalPaneMenuProps) {
  return (
<div
          className="absolute inset-0 z-[60]"
          onClick={closePaneMenu}
          onContextMenu={(event) => {
            event.preventDefault();
            closePaneMenu();
          }}
        >
          <div
            className="absolute min-w-[220px] rounded-lg border border-[var(--line)] bg-[var(--panel)] p-1 shadow-xl"
            style={{ left: paneMenu.x, top: paneMenu.y }}
            onClick={(event) => event.stopPropagation()}
          >
            <button
              type="button"
              className={paneMenu.hasSelection
                ? "flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
                : "flex w-full cursor-default items-center rounded px-3 py-1.5 text-left text-xs text-[var(--muted)] opacity-60"}
              disabled={!paneMenu.hasSelection}
              onClick={() => {
                void handleCopySelection();
              }}
            >
              Copy Selection
            </button>
            <button
              type="button"
              className="flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
              onClick={() => {
                void handlePaste();
              }}
            >
              Paste
            </button>
            <button
              type="button"
              className="flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
              onClick={handleSelectAll}
            >
              Select All
            </button>
            <button
              type="button"
              className="flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
              onClick={handleOpenSearch}
            >
              Find
            </button>
            <button
              type="button"
              className="flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
              onClick={handleClearScrollback}
            >
              Clear Scrollback
            </button>
            <div className="my-1 border-t border-[var(--line)]" />
            {isConnected ? (
              <button
                type="button"
                className="flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--bad)] transition-colors hover:bg-[var(--bad-glow)]"
                onClick={handleDisconnect}
              >
                Disconnect
              </button>
            ) : (
              <button
                type="button"
                className={canReconnect
                  ? "flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
                  : "flex w-full cursor-default items-center rounded px-3 py-1.5 text-left text-xs text-[var(--muted)] opacity-60"}
                disabled={!canReconnect}
                onClick={handleReconnectFromMenu}
              >
                Reconnect
              </button>
            )}
          </div>
        </div>
  );
}
