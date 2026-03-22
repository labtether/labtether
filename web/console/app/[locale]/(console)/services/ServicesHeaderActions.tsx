"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { ChevronDown, Pencil, RefreshCw, Settings } from "lucide-react";
import { Link } from "../../../../i18n/navigation";

interface ServicesHeaderActionsProps {
  selectionMode: boolean;
  layoutMode: boolean;
  bulkEditTargetCount: number;
  bulkSaving: boolean;
  syncing: boolean;
  pullingImages: boolean;
  pullPlanItemCount: number;
  onAddManual: () => void | Promise<void>;
  onToggleSelectionMode: () => void;
  onOpenBulkEdit: () => void;
  onToggleArrange: () => void;
  onRefresh: () => void | Promise<void>;
  onPullImages: () => void | Promise<void>;
}

export function ServicesHeaderActions({
  selectionMode,
  layoutMode,
  bulkEditTargetCount,
  bulkSaving,
  syncing,
  pullingImages,
  pullPlanItemCount,
  onAddManual,
  onToggleSelectionMode,
  onOpenBulkEdit,
  onToggleArrange,
  onRefresh,
  onPullImages,
}: ServicesHeaderActionsProps) {
  const t = useTranslations("services");
  const [editMenuOpen, setEditMenuOpen] = useState(false);
  const editMenuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!editMenuOpen) {
      return;
    }

    const closeOnOutsidePointer = (event: MouseEvent | TouchEvent) => {
      const menuHost = editMenuRef.current;
      const target = event.target;
      if (!(target instanceof Node)) {
        setEditMenuOpen(false);
        return;
      }
      if (menuHost && menuHost.contains(target)) {
        return;
      }
      setEditMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setEditMenuOpen(false);
      }
    };

    window.addEventListener("mousedown", closeOnOutsidePointer, true);
    window.addEventListener("touchstart", closeOnOutsidePointer, true);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("mousedown", closeOnOutsidePointer, true);
      window.removeEventListener("touchstart", closeOnOutsidePointer, true);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [editMenuOpen]);

  const menuItemClass =
    "w-full rounded px-2.5 py-1.5 text-left text-xs font-semibold text-[var(--text)] transition-colors hover:bg-[var(--hover)]";
  const menuItemDisabledClass =
    "w-full rounded px-2.5 py-1.5 text-left text-xs font-semibold text-[var(--muted)] opacity-60 cursor-not-allowed";

  return (
    <div className="flex max-w-full items-center justify-end gap-2 flex-wrap">
      <div ref={editMenuRef} className="relative">
        <button
          type="button"
          onClick={() => setEditMenuOpen((current) => !current)}
          aria-haspopup="menu"
          aria-expanded={editMenuOpen}
          className={`h-7 px-2.5 rounded border text-xs font-semibold transition-colors duration-[var(--dur-fast)] flex items-center gap-1.5 cursor-pointer ${
            editMenuOpen
              ? "border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--text)]"
              : "border-[var(--line)] text-[var(--text)] hover:bg-[var(--hover)]"
          }`}
        >
          <Pencil size={12} />
          {t("config.header.edit")}
          <ChevronDown size={11} className={editMenuOpen ? "rotate-180" : ""} />
        </button>

        {editMenuOpen ? (
          <div
            role="menu"
            className="absolute right-0 top-full z-30 mt-2 min-w-[210px] rounded-lg border border-[var(--line)] bg-[var(--panel)] p-1 shadow-xl"
          >
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                void onAddManual();
                setEditMenuOpen(false);
              }}
              className={menuItemClass}
            >
              {t("config.header.addManualService")}
            </button>
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                onToggleSelectionMode();
                setEditMenuOpen(false);
              }}
              className={menuItemClass}
            >
              {selectionMode ? t("config.header.doneSelect") : t("config.header.selectServices")}
            </button>
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                onOpenBulkEdit();
                setEditMenuOpen(false);
              }}
              disabled={bulkEditTargetCount === 0 || bulkSaving}
              className={
                bulkEditTargetCount === 0 || bulkSaving ? menuItemDisabledClass : menuItemClass
              }
            >
              {t("config.header.bulkEdit", { count: bulkEditTargetCount })}
            </button>
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                onToggleArrange();
                setEditMenuOpen(false);
              }}
              className={menuItemClass}
            >
              {layoutMode ? t("config.header.doneArranging") : t("config.header.arrangeServices")}
            </button>
          </div>
        ) : null}
      </div>

      <button
        type="button"
        onClick={() => void onRefresh()}
        disabled={syncing}
        className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-semibold text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)] flex items-center gap-1.5 cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
      >
        <RefreshCw size={12} className={syncing ? "animate-spin" : ""} />
        {syncing ? t("config.header.syncing") : t("config.header.refresh")}
      </button>
      <button
        type="button"
        onClick={() => void onPullImages()}
        disabled={pullingImages || pullPlanItemCount === 0}
        title={
          pullPlanItemCount > 0
            ? t("config.header.pullImagesTitle", { count: pullPlanItemCount })
            : t("config.header.noImagesAvailable")
        }
        className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-semibold text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)] flex items-center gap-1.5 cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
      >
        <RefreshCw size={12} className={pullingImages ? "animate-spin" : ""} />
        {pullingImages ? t("config.header.pulling") : t("config.header.pullImages", { count: pullPlanItemCount })}
      </button>
      <Link
        href="/services/config"
        className="inline-flex items-center justify-center h-8 w-8 rounded-md hover:bg-accent transition-colors"
        title={t("config.title")}
      >
        <Settings className="h-4 w-4" />
      </Link>
    </div>
  );
}
