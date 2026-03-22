"use client";

import { useState, useEffect, useMemo } from "react";
import { useFastStatus } from "../../contexts/StatusContext";
import { assetFreshnessLabel } from "../../console/formatters";
import { assetTypeIcon, isDeviceTier } from "../../console/taxonomy";
import type { Asset } from "../../console/models";
import { loadRecentTargets, type RecentTarget } from "./TerminalPane";
import { Search, Clock, Server, Box, Wifi, Zap } from "lucide-react";

interface ConnectionLandingProps {
  onSelectTarget: (nodeId: string) => void;
  onBrowseTargets: () => void;
  onQuickConnect?: () => void;
}

function formatRelativeTime(isoString: string): string {
  if (!isoString) return "";
  const now = Date.now();
  const then = new Date(isoString).getTime();
  if (Number.isNaN(then)) return "";
  const diffMs = now - then;
  const minutes = Math.floor(diffMs / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function freshnessDotColor(lastSeenAt: string): string {
  const freshness = assetFreshnessLabel(lastSeenAt);
  if (freshness === "online") return "var(--ok)";
  if (freshness === "unresponsive") return "var(--warn)";
  return "var(--muted)";
}

export default function ConnectionLanding({ onSelectTarget, onBrowseTargets, onQuickConnect }: ConnectionLandingProps) {
  const status = useFastStatus();
  const [recents, setRecents] = useState<RecentTarget[]>([]);

  useEffect(() => {
    setRecents(loadRecentTargets());
  }, []);

  const assetById = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const asset of status?.assets ?? []) {
      map.set(asset.id, asset);
    }
    return map;
  }, [status?.assets]);

  const resolvedRecents = useMemo(
    () =>
      recents
        .map((rt) => ({ recent: rt, asset: assetById.get(rt.id) }))
        .filter(
          (entry): entry is { recent: RecentTarget; asset: Asset } =>
            Boolean(entry.asset),
        ),
    [recents, assetById],
  );

  if (resolvedRecents.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 px-4">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-[var(--line)] bg-[var(--surface)]">
          <Wifi size={20} className="text-[var(--muted)]" />
        </div>
        <p className="text-sm text-[var(--muted)]">Select a target to start a session</p>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBrowseTargets}
            className="inline-flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-sm text-[var(--text)] transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
          >
            <Search size={14} />
            Browse all targets
          </button>
          {onQuickConnect ? (
            <button
              type="button"
              onClick={onQuickConnect}
              className="inline-flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-sm text-[var(--text)] transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
            >
              <Zap size={14} />
              Quick Connect
            </button>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col items-center justify-center gap-6 px-4">
      <div className="w-full max-w-md">
        <div className="mb-3 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
          <Clock size={11} />
          Recent Connections
        </div>
        <div className="grid grid-cols-1 gap-2">
          {resolvedRecents.map(({ recent, asset }) => {
            const Icon = assetTypeIcon(asset.type);
            const dotColor = freshnessDotColor(asset.last_seen_at);
            const isDevice = isDeviceTier(asset);
            return (
              <button
                key={asset.id}
                type="button"
                onClick={() => onSelectTarget(asset.id)}
                className="group flex items-center gap-3 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2.5 text-left transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
              >
                <span className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-[var(--line)] bg-[var(--panel)] text-[var(--muted)]">
                  <Icon size={15} />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-2">
                    <span
                      className="inline-block h-2 w-2 shrink-0 rounded-full"
                      style={{ backgroundColor: dotColor }}
                    />
                    <span className="truncate text-sm text-[var(--text)]">
                      {asset.name || asset.id}
                    </span>
                  </span>
                  <span className="mt-0.5 flex items-center gap-2 text-xs text-[var(--muted)]">
                    <span className="inline-flex items-center gap-1">
                      {isDevice ? <Server size={9} /> : <Box size={9} />}
                      {isDevice ? "Device" : "Container"}
                    </span>
                    {recent.lastConnected && (
                      <>
                        <span>·</span>
                        <span>{formatRelativeTime(recent.lastConnected)}</span>
                      </>
                    )}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex items-center gap-3 text-xs text-[var(--muted)]">
        <span className="h-px w-8 bg-[var(--line)]" />
        <span>or</span>
        <span className="h-px w-8 bg-[var(--line)]" />
      </div>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBrowseTargets}
          className="inline-flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-sm text-[var(--text)] transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
        >
          <Search size={14} />
          Browse all targets
        </button>
        {onQuickConnect ? (
          <button
            type="button"
            onClick={onQuickConnect}
            className="inline-flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-sm text-[var(--text)] transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
          >
            <Zap size={14} />
            Quick Connect
          </button>
        ) : null}
      </div>
    </div>
  );
}
