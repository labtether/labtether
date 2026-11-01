"use client";

import { useState, useEffect, useRef } from "react";
import { ChevronRight } from "lucide-react";
import { useTranslations } from "next-intl";
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
  return <div className="lt-menu-sep" role="separator" />;
}

function MenuItem({ label, onClick, danger }: { label: string; onClick: () => void; danger?: boolean }) {
  return (
    <button type="button" role="menuitem" onClick={onClick} className={`lt-menu-item mp-cm-item${danger ? " mp-danger" : ""}`}>
      {label}
    </button>
  );
}

function SubMenu({ label, children }: { label: string; children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mp-cm-sub" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button type="button" role="menuitem" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)} className="lt-menu-item mp-cm-item">
        {label}
        <ChevronRight aria-hidden="true" className="lt-menu-hint" />
      </button>
      {open && <div className="lt-menu mp-cm mp-cm-subm" role="menu">{children}</div>}
    </div>
  );
}

/** Edit layout: the right-click menus for the canvas, a zone, a device and a connection. */
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
  const t = useTranslations("topology");
  const menuRef = useRef<HTMLDivElement>(null);
  const [adjustedPos, setAdjustedPos] = useState<{ x: number; y: number } | null>(null);

  // Re-measure whenever the target changes, then keep the menu inside the window.
  useEffect(() => {
    setAdjustedPos(null);
  }, [target]);

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

  // Close on a click outside, Escape or scroll.
  useEffect(() => {
    if (!target) return;
    const handleKeyDown = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    const handleScroll = () => onClose();
    const handlePointerDown = (e: PointerEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) onClose();
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

  const wrap = (fn?: () => void) => () => { fn?.(); onClose(); };
  const wrapWith = <T,>(fn?: (arg: T) => void, arg?: T) => () => {
    if (fn && arg !== undefined) fn(arg);
    onClose();
  };

  return (
    <div
      ref={menuRef}
      role="menu"
      className="lt-menu mp-cm"
      style={{ left: (adjustedPos ?? target).x, top: (adjustedPos ?? target).y }}
    >
      {target.type === "canvas" && (
        <>
          <MenuItem label={t("edit.newZone")} onClick={wrap(onCreateZone)} />
          <MenuItem label={t("edit.fit")} onClick={wrap(onFitView)} />
          <MenuItem label={t("edit.arrange")} onClick={wrap(onAutoLayout)} />
          <Separator />
          <MenuItem label={t("menu.reset")} danger onClick={wrap(onResetLayout)} />
        </>
      )}

      {target.type === "zone" && (
        <>
          <MenuItem label={t("edit.rename")} onClick={wrapWith(onRenameZone, target.zoneId)} />
          <MenuItem label={t("edit.collapse")} onClick={wrapWith(onToggleCollapse, target.zoneId)} />
          <Separator />
          <MenuItem label={t("edit.deleteZone")} danger onClick={wrapWith(onDeleteZone, target.zoneId)} />
        </>
      )}

      {target.type === "asset" && (
        <>
          <MenuItem label={t("conn.connectTo")} onClick={wrapWith(onConnectTo, target.assetId)} />
          {zones.length > 0 && (
            <SubMenu label={t("edit.moveToZone")}>
              {zones.map((z) => (
                <MenuItem key={z.id} label={z.label} onClick={() => { onMoveToZone?.(target.assetId, z.id); onClose(); }} />
              ))}
            </SubMenu>
          )}
          <MenuItem label={t("edit.removeFromZone")} onClick={wrapWith(onRemoveFromZone, target.assetId)} />
        </>
      )}

      {target.type === "connection" && (
        <>
          <SubMenu label={t("edit.changeType")}>
            {RELATIONSHIP_TYPES.map((rt) => (
              <MenuItem key={rt.value} label={t(`rel.${rt.value}`)} onClick={() => { onChangeConnectionType?.(target.connectionId, rt.value); onClose(); }} />
            ))}
          </SubMenu>
          <Separator />
          <MenuItem label={t("edit.deleteConnection")} danger onClick={wrapWith(onDeleteConnection, target.connectionId)} />
        </>
      )}
    </div>
  );
}
