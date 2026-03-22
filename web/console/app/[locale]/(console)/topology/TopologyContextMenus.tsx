"use client";

import { useState, useEffect, useRef } from "react";
import type { Zone, RelationshipType } from "./topologyCanvasTypes";
import { RELATIONSHIP_TYPES } from "./topologyCanvasTypes";

export type ContextMenuTarget =
  | { type: "canvas"; x: number; y: number }
  | { type: "zone"; zoneId: string; label: string; x: number; y: number }
  | { type: "asset"; assetId: string; x: number; y: number }
  | { type: "connection"; connectionId: string; x: number; y: number };

interface TopologyContextMenuProps {
  target: ContextMenuTarget | null;
  zones: Zone[];
  onClose: () => void;
  // Canvas actions
  onCreateZone?: () => void;
  onFitView?: () => void;
  onAutoLayout?: () => void;
  onResetLayout?: () => void;
  // Zone actions
  onRenameZone?: (zoneId: string) => void;
  onDeleteZone?: (zoneId: string) => void;
  onToggleCollapse?: (zoneId: string) => void;
  // Asset actions
  onConnectTo?: (assetId: string) => void;
  onMoveToZone?: (assetId: string, zoneId: string) => void;
  onRemoveFromZone?: (assetId: string) => void;
  // Connection actions
  onChangeConnectionType?: (connId: string, type: RelationshipType) => void;
  onDeleteConnection?: (connId: string) => void;
}


function Separator() {
  return <div className="my-1 h-px bg-[var(--line)]" />;
}

interface MenuItemProps {
  icon: string;
  label: string;
  onClick: () => void;
  destructive?: boolean;
}

function MenuItem({ icon, label, onClick, destructive }: MenuItemProps) {
  return (
    <button
      onClick={onClick}
      className={`flex w-full items-center gap-2 rounded px-3 py-1.5 text-xs cursor-pointer transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)] ${
        destructive ? "text-[var(--bad)]" : "text-[var(--text)]"
      }`}
    >
      <span className="w-3.5 text-center leading-none">{icon}</span>
      <span className="flex-1 text-left">{label}</span>
    </button>
  );
}

interface SubMenuProps {
  icon: string;
  label: string;
  children: React.ReactNode;
}

function SubMenu({ icon, label, children }: SubMenuProps) {
  const [open, setOpen] = useState(false);

  return (
    <div
      className="relative"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      <button
        className="flex w-full items-center gap-2 rounded px-3 py-1.5 text-xs text-[var(--text)] cursor-pointer transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)]"
      >
        <span className="w-3.5 text-center leading-none">{icon}</span>
        <span className="flex-1 text-left">{label}</span>
        <span className="text-[var(--muted)]">›</span>
      </button>
      {open && (
        <div
          className="absolute left-full top-0 z-50 min-w-40 rounded-lg border border-[var(--panel-border)] bg-[var(--panel)] p-1 shadow-lg"
          style={{ marginLeft: 2 }}
        >
          {children}
        </div>
      )}
    </div>
  );
}

export function TopologyContextMenu({
  target,
  zones,
  onClose,
  onCreateZone,
  onFitView,
  onAutoLayout,
  onResetLayout,
  onRenameZone,
  onDeleteZone,
  onToggleCollapse,
  onConnectTo,
  onMoveToZone,
  onRemoveFromZone,
  onChangeConnectionType,
  onDeleteConnection,
}: TopologyContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [adjustedPos, setAdjustedPos] = useState<{ x: number; y: number } | null>(null);

  // Reset adjusted position when target changes so we always re-measure
  useEffect(() => {
    setAdjustedPos(null);
  }, [target]);

  // Clamp menu position to stay within viewport after it renders
  useEffect(() => {
    if (!target || !menuRef.current) return;
    const rect = menuRef.current.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let x = target.x;
    let y = target.y;
    if (x + rect.width > vw - 8) x = vw - rect.width - 8;
    if (y + rect.height > vh - 8) y = vh - rect.height - 8;
    if (x < 8) x = 8;
    if (y < 8) y = 8;
    setAdjustedPos({ x, y });
  }, [target]);

  // Close on outside click, Escape, or scroll
  useEffect(() => {
    if (!target) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };

    const handleScroll = () => onClose();

    const handlePointerDown = (e: PointerEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    document.addEventListener("pointerdown", handlePointerDown, true);
    document.addEventListener("scroll", handleScroll, true);

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.removeEventListener("pointerdown", handlePointerDown, true);
      document.removeEventListener("scroll", handleScroll, true);
    };
  }, [target, onClose]);

  if (!target) return null;

  const wrap = (fn?: () => void) => () => {
    fn?.();
    onClose();
  };

  const wrapWith = <T,>(fn?: (arg: T) => void, arg?: T) => () => {
    if (fn && arg !== undefined) fn(arg);
    onClose();
  };

  return (
    <div
      ref={menuRef}
      className="fixed z-[9999] min-w-44 rounded-lg border border-[var(--panel-border)] bg-[var(--panel)] p-1 shadow-lg"
      style={{ left: (adjustedPos ?? target).x, top: (adjustedPos ?? target).y }}
    >
      {target.type === "canvas" && (
        <>
          <MenuItem
            icon="+"
            label="New Zone"
            onClick={wrap(onCreateZone)}
          />
          <MenuItem
            icon="⊡"
            label="Fit View"
            onClick={wrap(onFitView)}
          />
          <MenuItem
            icon="⟳"
            label="Auto-layout"
            onClick={wrap(onAutoLayout)}
          />
          <Separator />
          <MenuItem
            icon="↺"
            label="Reset Layout"
            destructive
            onClick={() => {
              if (window.confirm("Reset the entire topology layout? All zones, placements, and dismissed assets will be cleared and re-seeded from your current infrastructure.")) {
                onResetLayout?.();
              }
              onClose();
            }}
          />
        </>
      )}

      {target.type === "zone" && (
        <>
          <MenuItem
            icon="✎"
            label="Rename"
            onClick={wrapWith(onRenameZone, target.zoneId)}
          />
          <MenuItem
            icon="⊟"
            label="Collapse / Expand"
            onClick={wrapWith(onToggleCollapse, target.zoneId)}
          />
          <Separator />
          <MenuItem
            icon="✕"
            label="Delete Zone"
            destructive
            onClick={wrapWith(onDeleteZone, target.zoneId)}
          />
        </>
      )}

      {target.type === "asset" && (
        <>
          <MenuItem
            icon="↔"
            label="Connect to..."
            onClick={wrapWith(onConnectTo, target.assetId)}
          />
          {zones.length > 0 && (
            <SubMenu icon="⤷" label="Move to zone">
              {zones.map((z) => (
                <button
                  key={z.id}
                  onClick={() => {
                    onMoveToZone?.(target.assetId, z.id);
                    onClose();
                  }}
                  className="flex w-full items-center gap-2 rounded px-3 py-1.5 text-xs text-[var(--text)] cursor-pointer transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)]"
                >
                  {z.label}
                </button>
              ))}
            </SubMenu>
          )}
          <MenuItem
            icon="⊖"
            label="Remove from zone"
            onClick={wrapWith(onRemoveFromZone, target.assetId)}
          />
        </>
      )}

      {target.type === "connection" && (
        <>
          <SubMenu icon="⇄" label="Change type">
            {RELATIONSHIP_TYPES.map((rt) => (
              <button
                key={rt.value}
                onClick={() => {
                  onChangeConnectionType?.(target.connectionId, rt.value);
                  onClose();
                }}
                className="flex w-full items-center gap-2 rounded px-3 py-1.5 text-xs text-[var(--text)] cursor-pointer transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)]"
              >
                {rt.label}
              </button>
            ))}
          </SubMenu>
          <Separator />
          <MenuItem
            icon="✕"
            label="Delete Connection"
            destructive
            onClick={wrapWith(onDeleteConnection, target.connectionId)}
          />
        </>
      )}
    </div>
  );
}
