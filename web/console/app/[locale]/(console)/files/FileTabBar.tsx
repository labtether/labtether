"use client";

import { Plus, Columns2, X } from "lucide-react";
import type { FileTab } from "./useFileTabsState";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface FileTabBarProps {
  tabs: FileTab[];
  activeTabId: string;
  splitMode: boolean;
  onAddTab: () => void;
  onRemoveTab: (tabId: string) => void;
  onSetActiveTab: (tabId: string) => void;
  onToggleSplit: () => void;
}

// ---------------------------------------------------------------------------
// Protocol dot color mapping
// ---------------------------------------------------------------------------

const PROTOCOL_DOT_COLOR: Record<string, string> = {
  agent:  "bg-green-500",
  sftp:   "bg-blue-500",
  smb:    "bg-orange-500",
  ftp:    "bg-teal-500",
  webdav: "bg-cyan-500",
};

function protocolDotClass(protocol: string | undefined): string {
  if (!protocol) return "bg-[var(--muted)]";
  return PROTOCOL_DOT_COLOR[protocol] ?? "bg-[var(--muted)]";
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function FileTabBar({
  tabs,
  activeTabId,
  splitMode,
  onAddTab,
  onRemoveTab,
  onSetActiveTab,
  onToggleSplit,
}: FileTabBarProps) {
  return (
    <div className="flex items-center gap-0.5 px-2 py-1 border-b border-[var(--line)] bg-[var(--panel)] overflow-x-auto">
      {/* Tabs */}
      {tabs.map((tab) => {
        const isActive = tab.id === activeTabId;

        return (
          <button
            key={tab.id}
            className={`group flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-xs whitespace-nowrap transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none select-none ${
              isActive
                ? "bg-[var(--surface)] text-[var(--text)] font-medium"
                : "text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]"
            }`}
            onClick={() => onSetActiveTab(tab.id)}
          >
            {/* Protocol dot */}
            <span
              className={`w-2 h-2 rounded-full flex-shrink-0 ${protocolDotClass(tab.protocol)}`}
            />

            {/* Label */}
            <span className="truncate max-w-[140px]" title={tab.label}>{tab.label}</span>

            {/* Close button */}
            <span
              role="button"
              tabIndex={0}
              className={`p-0.5 rounded transition-colors ${
                isActive
                  ? "text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)]"
                  : "opacity-0 group-hover:opacity-100 text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)]"
              }`}
              onClick={(event) => {
                event.stopPropagation();
                onRemoveTab(tab.id);
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.stopPropagation();
                  onRemoveTab(tab.id);
                }
              }}
            >
              <X className="w-3 h-3" />
            </span>
          </button>
        );
      })}

      {/* Add tab button */}
      <button
        className="flex items-center justify-center p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none flex-shrink-0"
        onClick={onAddTab}
        title="New tab"
      >
        <Plus className="w-3.5 h-3.5" />
      </button>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Split toggle button */}
      <button
        className={`flex items-center gap-1.5 px-2 py-1.5 rounded-md text-xs transition-colors duration-[var(--dur-instant)] cursor-pointer bg-transparent border-none flex-shrink-0 select-none ${
          splitMode
            ? "text-[var(--accent)] bg-[rgba(var(--accent-rgb),0.08)]"
            : "text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]"
        }`}
        onClick={onToggleSplit}
        title={splitMode ? "Exit split view" : "Split view"}
      >
        <Columns2 className="w-3.5 h-3.5" />
        <span>Split</span>
      </button>
    </div>
  );
}
