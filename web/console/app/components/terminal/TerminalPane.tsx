"use client";

import { memo, useCallback, useEffect, useMemo, useRef, useState, type MouseEvent, type RefObject } from "react";
import { useSession } from "../../hooks/useSession";
import type { QuickConnectParams } from "../../hooks/useSession";
import { useFastStatus } from "../../contexts/StatusContext";
import XTerminal from "../XTerminal";
import SearchBar from "./SearchBar";
import ConnectionLanding from "./ConnectionLanding";
import QuickConnectDialog from "./QuickConnectDialog";
import type { TerminalClipboardActionResult, XTerminalHandle } from "../XTerminal";
import type { TerminalPreferences } from "../../hooks/useTerminalPreferences";
import type { TerminalThemeDef } from "../../terminal/themes";
import type { TerminalFontDef } from "../../terminal/fonts";
import { assetFreshnessLabel } from "../../console/formatters";
import type { Asset } from "../../console/models";
import {
  assetTypeIcon,
  childParentKey,
  friendlyTypeLabel,
  hostParentKey,
  isDeviceTier,
  isHiddenAsset,
  isInfraHost,
} from "../../console/taxonomy";
import { SegmentedTabs } from "../ui/SegmentedTabs";
import {
  Box,
  Check,
  ChevronDown,
  History,
  RotateCcw,
  Search,
  Server,
  Wifi,
  WifiOff,
} from "lucide-react";

interface TerminalPaneProps {
  paneIndex: number;
  targetNodeId: string;
  onTargetChange: (nodeId: string) => void;
  isTabActive: boolean;
  isFocused: boolean;
  onFocus: () => void;
  broadcastActive: boolean;
  onBroadcastData: (data: string, sourcePaneIndex: number) => void;
  onTerminalRef: (handle: XTerminalHandle | null) => void;
  prefs: TerminalPreferences;
  themeDef: TerminalThemeDef;
  fontDef: TerminalFontDef;
  /** Optional: bypass target selection and attach to an existing persistent session. */
  initialPersistentSessionId?: string;
  /** Optional: pre-issued stream ticket for direct WebSocket connection. */
  initialStreamTicket?: string;
}

type TargetKind = "devices" | "containers";

const containerLikeTypes = new Set([
  "container",
  "docker-container",
  "pod",
  "deployment",
  "app",
  "stack",
  "compose-stack",
]);

export const recentsStorageKey = "labtether.terminal.recentTargets";

type PaneContextMenuState = {
  x: number;
  y: number;
  hasSelection: boolean;
};

type ClipboardNotice = {
  id: number;
  message: string;
};

export interface RecentTarget {
  id: string;
  name: string;
  type: "device" | "container";
  lastConnected: string; // ISO 8601
}

export function loadRecentTargets(): RecentTarget[] {
  try {
    const raw = window.localStorage.getItem(recentsStorageKey);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // Migrate old format (string[]) to new format (RecentTarget[])
    return parsed
      .map((entry: unknown) => {
        if (typeof entry === "string") {
          return { id: entry, name: "", type: "device" as const, lastConnected: "" };
        }
        if (entry && typeof entry === "object" && "id" in entry) {
          const obj = entry as Record<string, unknown>;
          return {
            id: String(obj.id ?? ""),
            name: String(obj.name ?? ""),
            type: obj.type === "container" ? ("container" as const) : ("device" as const),
            lastConnected: String(obj.lastConnected ?? ""),
          };
        }
        return null;
      })
      .filter((entry: RecentTarget | null): entry is RecentTarget => entry !== null && entry.id !== "")
      .slice(0, 8);
  } catch {
    return [];
  }
}

export function saveRecentTarget(target: RecentTarget): RecentTarget[] {
  const current = loadRecentTargets();
  const updated = [target, ...current.filter((entry) => entry.id !== target.id)].slice(0, 8);
  try {
    window.localStorage.setItem(recentsStorageKey, JSON.stringify(updated));
  } catch {
    // Ignore storage write failures.
  }
  return updated;
}

function clampMenuPosition(
  x: number,
  y: number,
  bounds?: { width: number; height: number },
  width = 220,
  height = 280,
  padding = 8,
) {
  const maxWidth = bounds?.width ?? (typeof window !== "undefined" ? window.innerWidth : width + padding * 2);
  const maxHeight = bounds?.height ?? (typeof window !== "undefined" ? window.innerHeight : height + padding * 2);
  return {
    x: Math.max(padding, Math.min(x, maxWidth - width - padding)),
    y: Math.max(padding, Math.min(y, maxHeight - height - padding)),
  };
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

function formatElapsed(ms: number): string {
  const safe = Number.isFinite(ms) ? Math.max(0, ms) : 0;
  if (safe < 1000) return `${safe}ms`;
  const seconds = safe / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const rem = Math.floor(seconds % 60);
  return `${minutes}m ${rem}s`;
}

function isContainerTarget(asset: Asset): boolean {
  return containerLikeTypes.has(asset.type);
}

function freshnessBadgeClass(lastSeenAt: string): string {
  const freshness = assetFreshnessLabel(lastSeenAt);
  if (freshness === "online") return "border-[var(--ok)]/35 bg-[var(--ok-glow)] text-[var(--ok)]";
  if (freshness === "unresponsive") return "border-[var(--warn)]/35 bg-[var(--warn-glow)] text-[var(--warn)]";
  if (freshness === "offline") return "border-[var(--bad)]/35 bg-[var(--bad-glow)] text-[var(--bad)]";
  return "border-[var(--line)] bg-[var(--surface)] text-[var(--muted)]";
}

interface TargetPickerProps {
  assets: Asset[];
  selectedTargetID: string;
  connectedAgentIDs: Set<string>;
  onSelect: (id: string) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface TargetRowProps {
  asset: Asset;
  showHost?: boolean;
  selectedTargetID: string;
  connectedAgentIDs: Set<string>;
  kind: TargetKind;
  containerHostByID: Map<string, string>;
  onSelect: (id: string) => void;
}

const TargetRow = memo(function TargetRow({
  asset,
  showHost = false,
  selectedTargetID,
  connectedAgentIDs,
  kind,
  containerHostByID,
  onSelect,
}: TargetRowProps) {
  const Icon = assetTypeIcon(asset.type);
  const isSelected = asset.id === selectedTargetID;
  const freshness = assetFreshnessLabel(asset.last_seen_at);
  const hasAgent = connectedAgentIDs.has(asset.id);
  const hostLabel = showHost ? containerHostByID.get(asset.id) ?? "Unassigned host" : "";
  const meta =
    kind === "containers"
      ? `${friendlyTypeLabel(asset.type)} on ${hostLabel} • ${asset.source}`
      : [friendlyTypeLabel(asset.type), asset.platform, asset.source]
          .filter(Boolean)
          .join(" • ");

  return (
    <button
      type="button"
      onClick={() => onSelect(asset.id)}
      className={`group flex w-full items-start gap-3 rounded-lg border px-3 py-2 text-left transition-colors ${
        isSelected
          ? "border-[var(--accent)]/40 bg-[var(--accent-subtle)]"
          : "border-[var(--line)] bg-[var(--surface)] hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
      }`}
    >
      <span className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-[var(--line)] bg-[var(--panel)] text-[var(--muted)]">
        <Icon size={15} />
      </span>
      <span className="min-w-0 flex-1 pt-0.5">
        <span className="block truncate text-sm text-[var(--text)]">{asset.name || asset.id}</span>
        <span className="block truncate text-xs text-[var(--muted)]">{meta}</span>
      </span>
      <span className="flex shrink-0 flex-col items-end gap-1">
        <span
          className={`rounded-full border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.05em] ${freshnessBadgeClass(asset.last_seen_at)}`}
        >
          {freshness}
        </span>
        {hasAgent ? (
          <span className="rounded-full border border-[var(--accent)]/35 bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--accent-text)]">
            agent
          </span>
        ) : null}
        {isSelected ? (
          <span className="inline-flex items-center gap-1 rounded-full border border-[var(--accent)]/35 bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--accent-text)]">
            <Check size={9} />
            selected
          </span>
        ) : null}
      </span>
    </button>
  );
});

function TargetPicker({
  assets,
  selectedTargetID,
  connectedAgentIDs,
  onSelect,
  open,
  onOpenChange,
}: TargetPickerProps) {
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<TargetKind>("devices");
  const [recentTargetData, setRecentTargetData] = useState<RecentTarget[]>([]);

  const byID = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const asset of assets) {
      map.set(asset.id, asset);
    }
    return map;
  }, [assets]);

  const selectedTarget = selectedTargetID ? byID.get(selectedTargetID) ?? null : null;
  const selectedKind: TargetKind =
    selectedTarget && isContainerTarget(selectedTarget) ? "containers" : "devices";

  const devices = useMemo(
    () => assets.filter((asset) => isDeviceTier(asset)).sort((a, b) => a.name.localeCompare(b.name)),
    [assets],
  );
  const containers = useMemo(
    () => assets.filter((asset) => isContainerTarget(asset)).sort((a, b) => a.name.localeCompare(b.name)),
    [assets],
  );

  useEffect(() => {
    setRecentTargetData(loadRecentTargets());
  }, []);

  useEffect(() => {
    if (!open) return undefined;

    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onOpenChange(false);
      }
    };

    window.addEventListener("keydown", onEscape);
    return () => {
      window.removeEventListener("keydown", onEscape);
    };
  }, [open, onOpenChange]);

  useEffect(() => {
    if (open) {
      setKind(selectedKind);
      const focusTimer = window.setTimeout(() => searchInputRef.current?.focus(), 0);
      return () => window.clearTimeout(focusTimer);
    }
    if (!open) {
      setQuery("");
    }
  }, [open, selectedKind]);

  const rememberRecentTarget = useCallback((asset: Asset) => {
    const target: RecentTarget = {
      id: asset.id,
      name: asset.name || asset.id,
      type: isContainerTarget(asset) ? "container" : "device",
      lastConnected: new Date().toISOString(),
    };
    setRecentTargetData(saveRecentTarget(target));
  }, []);

  const queryValue = query.trim().toLowerCase();
  const matchesQuery = useCallback(
    (asset: Asset) => {
      if (!queryValue) return true;
      const fields = [
        asset.name,
        asset.id,
        friendlyTypeLabel(asset.type),
        asset.type,
        asset.source,
        asset.platform ?? "",
        ...(asset.tags ?? []),
        ...(asset.metadata ? Object.values(asset.metadata) : []),
      ];
      return fields.join(" ").toLowerCase().includes(queryValue);
    },
    [queryValue],
  );

  const visibleTargets = useMemo(() => {
    const pool = kind === "devices" ? devices : containers;
    return pool.filter(matchesQuery);
  }, [containers, devices, kind, matchesQuery]);

  const resolvedRecentTargets = useMemo(
    () =>
      recentTargetData
        .map((rt) => byID.get(rt.id))
        .filter((asset): asset is Asset => Boolean(asset))
        .filter((asset) => {
          const targetKind: TargetKind = isContainerTarget(asset) ? "containers" : "devices";
          return targetKind === kind && matchesQuery(asset);
        }),
    [recentTargetData, byID, kind, matchesQuery],
  );

  const recentTargetSet = useMemo(
    () => new Set(resolvedRecentTargets.map((asset) => asset.id)),
    [resolvedRecentTargets],
  );
  const sectionTargets = useMemo(
    () => visibleTargets.filter((asset) => !recentTargetSet.has(asset.id)),
    [recentTargetSet, visibleTargets],
  );
  const totalVisible = visibleTargets.length;

  const containerHostByID = useMemo(() => {
    const infraHostByParentKey = new Map<string, Asset>();
    for (const asset of devices) {
      if (!isInfraHost(asset)) continue;
      infraHostByParentKey.set(hostParentKey(asset), asset);
    }

    const hostByContainer = new Map<string, string>();
    for (const container of containers) {
      const parentKey = childParentKey(container);
      const host = parentKey ? infraHostByParentKey.get(parentKey) : undefined;
      const fallbackHost =
        container.metadata?.node ||
        container.metadata?.host ||
        container.metadata?.endpoint_name ||
        container.metadata?.endpoint_id ||
        "Unassigned host";
      hostByContainer.set(container.id, host?.name || fallbackHost);
    }
    return hostByContainer;
  }, [containers, devices]);

  const groupedContainers = useMemo(() => {
    if (kind !== "containers") return [];
    const grouped = new Map<string, Asset[]>();
    for (const asset of sectionTargets) {
      const hostLabel = containerHostByID.get(asset.id) ?? "Unassigned host";
      const list = grouped.get(hostLabel) ?? [];
      list.push(asset);
      grouped.set(hostLabel, list);
    }
    return Array.from(grouped.entries())
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([hostLabel, hostTargets]) => ({
        hostLabel,
        targets: hostTargets.sort((a, b) => a.name.localeCompare(b.name)),
      }));
  }, [containerHostByID, kind, sectionTargets]);

  const handleSelectTarget = useCallback(
    (id: string) => {
      if (id === selectedTargetID) {
        onOpenChange(false);
        return;
      }
      onSelect(id);
      onOpenChange(false);
      const asset = byID.get(id);
      if (asset) rememberRecentTarget(asset);
    },
    [byID, onOpenChange, onSelect, rememberRecentTarget, selectedTargetID],
  );

  const kindOptions = useMemo(
    () => [
      {
        id: "devices" as const,
        label: (
          <span className="inline-flex items-center gap-1.5">
            <Server size={12} />
            <span>Devices</span>
            <span className="text-[10px] text-[var(--muted)]">{devices.length}</span>
          </span>
        ),
        ariaLabel: "Show device targets",
      },
      {
        id: "containers" as const,
        label: (
          <span className="inline-flex items-center gap-1.5">
            <Box size={12} />
            <span>Containers</span>
            <span className="text-[10px] text-[var(--muted)]">{containers.length}</span>
          </span>
        ),
        ariaLabel: "Show container targets",
      },
    ],
    [containers.length, devices.length],
  );

  return (
    <div className="min-w-0 flex-1">
      <button
        type="button"
        onClick={() => onOpenChange(true)}
        className="flex h-9 w-full min-w-0 items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--panel)] px-2 transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
        title="Choose terminal target"
      >
        <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded border border-[var(--line)] bg-[var(--surface)] text-[var(--muted)]">
          {selectedTarget ? (
            (() => {
              const Icon = assetTypeIcon(selectedTarget.type);
              return <Icon size={13} />;
            })()
          ) : (
            <Search size={13} />
          )}
        </span>
        <span className="min-w-0 flex-1 text-left">
          <span className="block truncate text-xs text-[var(--text)]">
            {selectedTarget ? selectedTarget.name || selectedTarget.id : "Select target"}
          </span>
          <span className="block truncate text-[10px] uppercase tracking-[0.06em] text-[var(--muted)]">
            {selectedTarget
              ? `${selectedKind === "devices" ? "Device" : "Container"} target`
              : "Choose Device or Container"}
          </span>
        </span>
        <ChevronDown
          size={14}
          className={`shrink-0 text-[var(--muted)] transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>

      {open ? (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Choose terminal target"
          className="fixed inset-0 z-[70] flex items-start justify-center p-4 pt-[8vh]"
        >
          <button
            type="button"
            aria-label="Close target picker"
            onClick={() => onOpenChange(false)}
            className="absolute inset-0 bg-black/76"
          />

          <div className="relative z-10 w-full max-w-[56rem] overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--panel)] shadow-[var(--shadow-lg)]">
            <div className="border-b border-[var(--line)] p-4">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                    Connect Terminal
                  </p>
                  <p className="mt-1 text-sm font-semibold text-[var(--text)]">
                    Pick a target type first, then choose the endpoint
                  </p>
                  <p className="mt-0.5 text-xs text-[var(--muted)]">
                    Devices open host shells. Containers open workload shells.
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => onOpenChange(false)}
                  className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--muted)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--text)]"
                >
                  Done
                </button>
              </div>

              <div className="mt-3 grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
                <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-1">
                  <SegmentedTabs
                    value={kind}
                    options={kindOptions}
                    onChange={setKind}
                    size="sm"
                  />
                </div>
                <div className="relative">
                  <Search
                    size={14}
                    className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--muted)]"
                  />
                  <input
                    ref={searchInputRef}
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder={
                      kind === "devices"
                        ? "Search devices by name, type, platform, or tag..."
                        : "Search containers by name, host, source, or tag..."
                    }
                    className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] py-2 pl-8 pr-3 text-sm text-[var(--text)] outline-none transition-colors focus:border-[var(--accent)]"
                  />
                </div>
              </div>

              <div className="mt-2 flex items-center justify-between text-xs text-[var(--muted)]">
                <span>{kind === "devices" ? "Devices" : "Containers"} visible</span>
                <span>{totalVisible} result{totalVisible === 1 ? "" : "s"}</span>
              </div>
            </div>

            <div className="max-h-[min(64vh,38rem)] overflow-y-auto p-4">
              {resolvedRecentTargets.length > 0 ? (
                <div className="mb-4">
                  <div className="mb-2 flex items-center gap-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                    <History size={11} />
                    Recent {kind === "devices" ? "Devices" : "Containers"}
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {resolvedRecentTargets.map((asset) => (
                      <TargetRow
                        key={`recent-${asset.id}`}
                        asset={asset}
                        showHost={kind === "containers"}
                        selectedTargetID={selectedTargetID}
                        connectedAgentIDs={connectedAgentIDs}
                        kind={kind}
                        containerHostByID={containerHostByID}
                        onSelect={handleSelectTarget}
                      />
                    ))}
                  </div>
                </div>
              ) : null}

              {kind === "devices" && sectionTargets.length > 0 ? (
                <div>
                  <div className="mb-2 flex items-center gap-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                    <Server size={11} />
                    Devices
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {sectionTargets.map((asset) => (
                      <TargetRow
                        key={asset.id}
                        asset={asset}
                        selectedTargetID={selectedTargetID}
                        connectedAgentIDs={connectedAgentIDs}
                        kind={kind}
                        containerHostByID={containerHostByID}
                        onSelect={handleSelectTarget}
                      />
                    ))}
                  </div>
                </div>
              ) : null}

              {kind === "containers" && groupedContainers.length > 0 ? (
                <div className="space-y-3">
                  {groupedContainers.map((group) => (
                    <div key={group.hostLabel}>
                      <div className="mb-2 flex items-center justify-between text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                        <span className="truncate">{group.hostLabel}</span>
                        <span>{group.targets.length}</span>
                      </div>
                      <div className="grid grid-cols-1 gap-2">
                        {group.targets.map((asset) => (
                          <TargetRow
                            key={asset.id}
                            asset={asset}
                            showHost
                            selectedTargetID={selectedTargetID}
                            connectedAgentIDs={connectedAgentIDs}
                            kind={kind}
                            containerHostByID={containerHostByID}
                            onSelect={handleSelectTarget}
                          />
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}

              {totalVisible === 0 ? (
                <div className="rounded-lg border border-dashed border-[var(--line)] bg-[var(--surface)] px-4 py-8 text-center text-sm text-[var(--muted)]">
                  {kind === "devices"
                    ? "No devices match your search."
                    : "No containers match your search."}
                </div>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export default function TerminalPane({
  paneIndex,
  targetNodeId,
  onTargetChange,
  isTabActive,
  isFocused,
  onFocus,
  broadcastActive,
  onBroadcastData,
  onTerminalRef,
  prefs,
  themeDef,
  fontDef,
  initialPersistentSessionId: _initialPersistentSessionId,
  initialStreamTicket: _initialStreamTicket,
}: TerminalPaneProps) {
  const [quickConnectParams, setQuickConnectParams] = useState<QuickConnectParams | undefined>(undefined);
  const [quickConnectOpen, setQuickConnectOpen] = useState(false);
  const session = useSession({ type: "terminal", autoReconnect: quickConnectParams ? false : prefs.auto_reconnect, quickConnectParams });
  const status = useFastStatus();
  const termRef = useRef<XTerminalHandle | null>(null);
  const paneMenuHostRef = useRef<HTMLDivElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [wasConnected, setWasConnected] = useState(false);
  const [targetPickerOpen, setTargetPickerOpen] = useState(false);
  const [paneMenu, setPaneMenu] = useState<PaneContextMenuState | null>(null);
  const [clipboardNotice, setClipboardNotice] = useState<ClipboardNotice | null>(null);
  const setTargetRef = useRef(session.setTarget);
  const connectRef = useRef(session.connect);

  const assets = status?.assets;
  const visibleTargets = useMemo(
    () => (assets ?? []).filter((asset) => !isHiddenAsset(asset)),
    [assets],
  );

  useEffect(() => {
    setTargetRef.current = session.setTarget;
  }, [session.setTarget]);

  useEffect(() => {
    connectRef.current = session.connect;
  }, [session.connect]);

  useEffect(() => {
    if (targetNodeId) {
      setTargetRef.current(targetNodeId);
    }
  }, [targetNodeId]);

  useEffect(() => {
    if (isTabActive && targetNodeId && session.connectionState === "idle" && !wasConnected) {
      void connectRef.current(targetNodeId);
    }
  }, [isTabActive, targetNodeId, session.connectionState, wasConnected]);

  const handleConnected = useCallback(() => {
    setWasConnected(true);
    session.handleConnected();
  }, [session]);

  const handleStreamReady = useCallback((message?: string) => {
    setWasConnected(true);
    session.handleStreamReady(message);
  }, [session]);

  const handleDisconnected = useCallback(
    (reason?: string) => {
      session.handleDisconnected(reason);
    },
    [session],
  );

  // Forward ref to parent.
  const setTermRef = useCallback(
    (el: XTerminalHandle | null) => {
      termRef.current = el;
      onTerminalRef(el);
    },
    [onTerminalRef],
  );

  const closePaneMenu = useCallback(() => {
    setPaneMenu(null);
  }, []);

  useEffect(() => {
    if (!clipboardNotice) return undefined;
    const timer = window.setTimeout(() => setClipboardNotice(null), 4200);
    return () => window.clearTimeout(timer);
  }, [clipboardNotice]);

  const handleClipboardActionResult = useCallback(
    (result: TerminalClipboardActionResult, action: "copy" | "paste") => {
      if (result.ok || result.reason !== "permission-denied") {
        return;
      }
      const message = action === "copy"
        ? "Clipboard write was blocked by the browser. Allow clipboard access or use your browser's native copy prompt."
        : "Clipboard read was blocked by the browser. Allow clipboard access and try paste again.";
      setClipboardNotice({ id: Date.now(), message });
    },
    [],
  );

  // Broadcast data capture: forward typed data to other panes.
  const handleDataCapture = useCallback(
    (data: string) => {
      if (broadcastActive && isFocused) {
        onBroadcastData(data, paneIndex);
      }
    },
    [broadcastActive, isFocused, onBroadcastData, paneIndex],
  );

  // Keyboard shortcuts for focused pane.
  useEffect(() => {
    if (!isFocused) return undefined;
    const handler = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) return;
      const key = event.key.toLowerCase();
      const hasPrimaryModifier = event.ctrlKey || event.metaKey;

      if (!hasPrimaryModifier) return;

      if (key === "f") {
        event.preventDefault();
        setSearchOpen((value) => !value);
        return;
      }

      if (key === "a" && !event.shiftKey) {
        event.preventDefault();
        termRef.current?.selectAll();
        return;
      }

      if (event.shiftKey && key === "c") {
        event.preventDefault();
        void termRef.current?.copySelection().then((result) => {
          if (result) handleClipboardActionResult(result, "copy");
        });
        return;
      }

      if (event.shiftKey && key === "v") {
        event.preventDefault();
        void termRef.current?.pasteFromClipboard().then((result) => {
          if (result) handleClipboardActionResult(result, "paste");
        });
      }
    };

    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [handleClipboardActionResult, isFocused]);

  useEffect(() => {
    if (!paneMenu) return undefined;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closePaneMenu();
      }
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [paneMenu, closePaneMenu]);

  const handleTargetSelect = useCallback(
    (newTarget: string) => {
      const trimmed = newTarget.trim();
      if (!trimmed || trimmed === targetNodeId) {
        return;
      }
      onTargetChange(trimmed);
      session.disconnect();
      setWasConnected(false);
      setTargetPickerOpen(false);
      void session.connect(trimmed);
    },
    [onTargetChange, session, targetNodeId],
  );

  const handleReconnect = useCallback(() => {
    if (targetNodeId) {
      void session.connect(targetNodeId);
    }
  }, [session, targetNodeId]);

  const handleQuickConnect = useCallback(
    (params: QuickConnectParams) => {
      setQuickConnectOpen(false);
      session.disconnect();
      setWasConnected(false);
      setQuickConnectParams(params);
      // connect() is triggered by useEffect below once quickConnectParams state settles.
    },
    [session],
  );

  // Trigger connect once when quickConnectParams are set.
  // Intentionally excludes session.connect from deps — we only want to fire
  // when quickConnectParams changes, not on every session state transition.
  const sessionConnectRef = useRef(session.connect);
  sessionConnectRef.current = session.connect;

  useEffect(() => {
    if (!quickConnectParams) return;
    void sessionConnectRef.current("quick-connect");
  }, [quickConnectParams]);

  const handlePaneContextMenu = useCallback((event: MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();
    onFocus();
    const hostRect = paneMenuHostRef.current?.getBoundingClientRect();
    const pos = clampMenuPosition(
      hostRect ? event.clientX - hostRect.left : event.clientX,
      hostRect ? event.clientY - hostRect.top : event.clientY,
      hostRect ? { width: hostRect.width, height: hostRect.height } : undefined,
    );
    setPaneMenu({
      x: pos.x,
      y: pos.y,
      hasSelection: termRef.current?.hasSelection() ?? false,
    });
  }, [onFocus]);

  const handleCopySelection = useCallback(async () => {
    const result = await termRef.current?.copySelection();
    if (result) handleClipboardActionResult(result, "copy");
    closePaneMenu();
    termRef.current?.focus();
  }, [closePaneMenu, handleClipboardActionResult]);

  const handlePaste = useCallback(async () => {
    const result = await termRef.current?.pasteFromClipboard();
    if (result) handleClipboardActionResult(result, "paste");
    closePaneMenu();
    termRef.current?.focus();
  }, [closePaneMenu, handleClipboardActionResult]);

  const handleSelectAll = useCallback(() => {
    termRef.current?.selectAll();
    closePaneMenu();
  }, [closePaneMenu]);

  const handleOpenSearch = useCallback(() => {
    setSearchOpen(true);
    closePaneMenu();
  }, [closePaneMenu]);

  const handleClearScrollback = useCallback(() => {
    termRef.current?.clearScrollback();
    closePaneMenu();
    termRef.current?.focus();
  }, [closePaneMenu]);

  const handleDisconnect = useCallback(() => {
    session.disconnect();
    closePaneMenu();
  }, [session, closePaneMenu]);

  const handleReconnectFromMenu = useCallback(() => {
    handleReconnect();
    closePaneMenu();
  }, [handleReconnect, closePaneMenu]);

  const isConnected = session.connectionState === "connected";
  const isDisconnected = session.connectionState === "idle" && wasConnected;
  const isError = session.connectionState === "error";

  const paneBorder = isFocused
    ? broadcastActive
      ? "1px solid var(--warn)"
      : "1px solid var(--accent)"
    : broadcastActive
      ? "1px solid var(--warn-glow)"
      : "1px solid var(--line)";

  const statusColor = isConnected
    ? "var(--ok)"
    : session.connectionState === "connecting"
      ? "var(--warn)"
      : isError
        ? "var(--bad)"
        : "var(--muted)";

  const showProgress = session.connectionState === "connecting" || session.connectionState === "authenticating";
  const progressLabel = `${session.connectionProgress.message} (${formatElapsed(session.connectionProgress.totalElapsedMs)})`;

  return (
    <div
      ref={paneMenuHostRef}
      onClick={onFocus}
      className="relative flex h-full min-h-0 flex-col overflow-hidden rounded-lg bg-[var(--panel)]"
      style={{ border: paneBorder }}
    >
      {broadcastActive ? (
        <div
          className="absolute left-0 right-0 top-0 z-[5] h-0.5"
          style={{ backgroundColor: "var(--warn)" }}
        />
      ) : null}

      {/* Pane header */}
      <div className="flex shrink-0 items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-2 py-1.5 text-xs">
        <TargetPicker
          assets={visibleTargets}
          selectedTargetID={targetNodeId}
          connectedAgentIDs={session.connectedAgentIds}
          onSelect={handleTargetSelect}
          open={targetPickerOpen}
          onOpenChange={setTargetPickerOpen}
        />

        <span
          className="inline-block h-2 w-2 rounded-full"
          style={{ backgroundColor: statusColor }}
          title={session.connectionState}
        />

        {broadcastActive ? (
          <span className="rounded-full border border-[var(--warn)]/45 bg-[var(--warn-glow)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--warn)]">
            Cast
          </span>
        ) : null}

        {showProgress ? (
          <span className="rounded-full border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-[10px] uppercase tracking-[0.06em] text-[var(--muted)]">
            {progressLabel}
          </span>
        ) : null}

        {isConnected ? (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              session.disconnect();
            }}
            className="inline-flex h-5 w-5 items-center justify-center rounded border border-transparent text-[var(--muted)] transition-colors hover:border-[var(--line)] hover:bg-[var(--hover)] hover:text-[var(--text)]"
            title="Disconnect"
          >
            <WifiOff size={11} />
          </button>
        ) : null}
      </div>

      {/* Terminal area */}
      <div
        className="relative min-h-0 flex-1"
        onContextMenu={handlePaneContextMenu}
      >
        <SearchBar
          termRef={termRef as RefObject<XTerminalHandle | null>}
          open={searchOpen}
          onClose={() => setSearchOpen(false)}
        />

        {session.wsUrl ? (
          <>
            <XTerminal
              ref={setTermRef}
              wsUrl={session.wsUrl}
              onConnected={handleConnected}
              onDisconnected={handleDisconnected}
              onError={session.handleError}
              onStreamStatus={session.handleStreamStatus}
              onStreamReady={handleStreamReady}
              theme={themeDef.theme}
              fontFamily={fontDef.family}
              fontSize={prefs.font_size}
              cursorStyle={prefs.cursor_style}
              cursorBlink={prefs.cursor_blink}
              scrollback={prefs.scrollback}
              onDataCapture={handleDataCapture}
            />
            {showProgress ? (
              <div className="pointer-events-none absolute left-3 top-3 z-[6] rounded-md border border-[var(--line)] bg-[var(--panel)] px-2 py-1 text-[10px] text-[var(--muted)] backdrop-blur-sm">
                {progressLabel}
              </div>
            ) : null}
          </>
        ) : (
          <div className="flex h-full min-h-[120px] flex-col items-center justify-center gap-3 px-3 text-center text-xs text-[var(--muted)]">
            {!targetNodeId && !quickConnectParams ? (
              <ConnectionLanding
                onSelectTarget={handleTargetSelect}
                onBrowseTargets={() => setTargetPickerOpen(true)}
                onQuickConnect={() => setQuickConnectOpen(true)}
              />
            ) : session.connectionState === "connecting" ? (
              <span>{progressLabel}</span>
            ) : isError ? (
              <>
                <span className="text-[var(--bad)]">{session.error || "Connection error"}</span>
                <button
                  type="button"
                  onClick={handleReconnect}
                  className="inline-flex items-center gap-1.5 rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
                >
                  <RotateCcw size={12} />
                  Retry
                </button>
              </>
            ) : targetNodeId && !wasConnected ? (
              <button
                type="button"
                onClick={handleReconnect}
                className="inline-flex items-center gap-1.5 rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
              >
                <Wifi size={12} />
                Connect
              </button>
            ) : null}
          </div>
        )}

        {/* Reconnect overlay */}
        {isDisconnected ? (
          <div className="absolute inset-0 z-[5] flex flex-col items-center justify-center gap-3 bg-black/65 px-3">
            <span className="text-xs text-[var(--text)]">Session disconnected</span>
            <button
              type="button"
              onClick={handleReconnect}
              className="inline-flex items-center gap-1.5 rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
            >
              <RotateCcw size={12} />
              Reconnect
            </button>
          </div>
        ) : null}

        {clipboardNotice ? (
          <div className="pointer-events-none absolute right-3 top-3 z-[7] max-w-[20rem]">
            <div
              key={clipboardNotice.id}
              role="status"
              aria-live="polite"
              className="rounded-md border border-[var(--warn)]/35 bg-[var(--panel)]/95 px-3 py-2 text-xs text-[var(--warn)] shadow-[var(--shadow-md)] backdrop-blur-sm"
            >
              {clipboardNotice.message}
            </div>
          </div>
        ) : null}

      </div>

      {paneMenu ? (
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
                className={targetNodeId
                  ? "flex w-full items-center rounded px-3 py-1.5 text-left text-xs text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
                  : "flex w-full cursor-default items-center rounded px-3 py-1.5 text-left text-xs text-[var(--muted)] opacity-60"}
                disabled={!targetNodeId}
                onClick={handleReconnectFromMenu}
              >
                Reconnect
              </button>
            )}
          </div>
        </div>
      ) : null}

      <QuickConnectDialog
        open={quickConnectOpen}
        onClose={() => setQuickConnectOpen(false)}
        onConnect={handleQuickConnect}
      />
    </div>
  );
}
