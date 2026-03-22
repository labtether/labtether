"use client";

import { X, Plus } from "lucide-react";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";
import { PROTOCOL_DOT_COLOR } from "./types";

interface RemoteViewTabBarProps {
  tabs: RemoteViewTab[];
  activeTabId: string;
  onAddTab: () => void;
  onRemoveTab: (tabId: string) => void;
  onSetActiveTab: (tabId: string) => void;
  connectionState?: RemoteViewConnectionState;
  latencyMs?: number | null;
}

function protocolDotClass(protocol: string | undefined): string {
  if (!protocol) return "bg-[var(--muted)]";
  return PROTOCOL_DOT_COLOR[protocol as keyof typeof PROTOCOL_DOT_COLOR] ?? "bg-[var(--muted)]";
}

function statusColor(state: RemoteViewConnectionState): string {
  switch (state) {
    case "connected":
      return "bg-[var(--ok)]";
    case "connecting":
    case "authenticating":
      return "bg-[var(--warn)] animate-pulse";
    case "error":
      return "bg-[var(--bad)]";
    default:
      return "bg-[var(--muted)]";
  }
}

function statusLabel(state: RemoteViewConnectionState): string {
  switch (state) {
    case "connected":
      return "Connected";
    case "connecting":
      return "Connecting";
    case "authenticating":
      return "Authenticating";
    case "disconnected":
      return "Disconnected";
    case "error":
      return "Error";
    default:
      return "Idle";
  }
}

export default function RemoteViewTabBar({
  tabs,
  activeTabId,
  onAddTab,
  onRemoveTab,
  onSetActiveTab,
  connectionState,
  latencyMs,
}: RemoteViewTabBarProps) {
  return (
    <div className="flex items-center gap-0.5 px-2 py-1 border-b border-[var(--line)] overflow-x-auto bg-[var(--surface)]">
      {tabs.map((tab) => {
        const isActive = tab.id === activeTabId;
        return (
          <button
            key={tab.id}
            onClick={() => onSetActiveTab(tab.id)}
            className={`group flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-xs whitespace-nowrap transition-colors duration-[var(--dur-fast)] ${
              isActive
                ? "bg-[var(--panel-glass)] text-[var(--text)] shadow-sm"
                : "text-[var(--text-secondary)] hover:bg-[var(--panel-glass)]/50"
            }`}
          >
            <span className={`w-2 h-2 rounded-full flex-shrink-0 ${protocolDotClass(tab.protocol)}`} />
            <span className="max-w-[140px] truncate">{tab.label}</span>
            <span
              role="button"
              tabIndex={-1}
              className={`ml-0.5 p-0.5 rounded hover:bg-[var(--hover)] ${
                isActive ? "opacity-60 hover:opacity-100" : "opacity-0 group-hover:opacity-60 hover:!opacity-100"
              }`}
              onClick={(e) => {
                e.stopPropagation();
                onRemoveTab(tab.id);
              }}
            >
              <X className="w-3 h-3" />
            </span>
          </button>
        );
      })}

      <button
        onClick={onAddTab}
        className="flex-shrink-0 p-1.5 rounded-md text-[var(--text-secondary)] hover:bg-[var(--panel-glass)]/50 transition-colors duration-[var(--dur-fast)]"
        title="New tab"
      >
        <Plus className="w-3.5 h-3.5" />
      </button>

      <div className="flex-1" />

      {connectionState && connectionState !== "idle" && (
        <div className="flex items-center gap-1.5 px-2 py-1 rounded-md bg-[var(--panel-glass)] text-xs flex-shrink-0">
          <span className={`w-1.5 h-1.5 rounded-full ${statusColor(connectionState)}`} />
          <span className="text-[var(--text-secondary)]">{statusLabel(connectionState)}</span>
          {connectionState === "connected" && latencyMs != null && (
            <span className="text-[var(--muted)] ml-1">{latencyMs}ms</span>
          )}
        </div>
      )}
    </div>
  );
}
