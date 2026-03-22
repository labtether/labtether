"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { X } from "lucide-react";

import { IconPicker } from "./IconPicker";
import { describeServiceIconSelection, ServiceIcon } from "./ServiceIcon";
import type { ServiceCustomIcon } from "../hooks/useWebServices";

const CATEGORIES = [
  "Media",
  "Downloads",
  "Gaming",
  "Networking",
  "Monitoring",
  "Home Automation",
  "Management",
  "Development",
  "Storage",
  "Databases",
  "Security",
  "Productivity",
  "Other",
];

const noChangeValue = "__no_change__";

export interface ServiceBulkEditChanges {
  category?: string;
  hidden?: boolean;
  iconMode: "no_change" | "set" | "clear";
  iconValue?: string;
}

interface ServiceBulkEditModalProps {
  open: boolean;
  affectedCount: number;
  icons: string[];
  customIcons: ServiceCustomIcon[];
  busy?: boolean;
  onApply: (changes: ServiceBulkEditChanges) => Promise<void>;
  onClose: () => void;
}

export function ServiceBulkEditModal({
  open,
  affectedCount,
  icons,
  customIcons,
  busy = false,
  onApply,
  onClose,
}: ServiceBulkEditModalProps) {
  const [categoryChoice, setCategoryChoice] = useState<string>(noChangeValue);
  const [visibilityChoice, setVisibilityChoice] = useState<string>(noChangeValue);
  const [iconMode, setIconMode] = useState<"no_change" | "set" | "clear">("no_change");
  const [iconValue, setIconValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const iconSelectionLabel = useMemo(() => describeServiceIconSelection(iconValue), [iconValue]);

  useEffect(() => {
    if (!open) {
      return;
    }
    setCategoryChoice(noChangeValue);
    setVisibilityChoice(noChangeValue);
    setIconMode("no_change");
    setIconValue("");
    setError(null);
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [busy, onClose, open]);

  const handleApply = useCallback(async () => {
    setError(null);

    const changes: ServiceBulkEditChanges = {
      iconMode,
    };
    if (categoryChoice !== noChangeValue) {
      changes.category = categoryChoice;
    }
    if (visibilityChoice === "hide") {
      changes.hidden = true;
    } else if (visibilityChoice === "unhide") {
      changes.hidden = false;
    }
    if (iconMode === "set") {
      if (!iconValue.trim()) {
        setError("Select an icon before applying.");
        return;
      }
      changes.iconValue = iconValue.trim();
    }

    if (
      changes.category === undefined &&
      changes.hidden === undefined &&
      changes.iconMode === "no_change"
    ) {
      setError("Choose at least one change to apply.");
      return;
    }

    try {
      await onApply(changes);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to apply bulk changes");
    }
  }, [categoryChoice, iconMode, iconValue, onApply, onClose, visibilityChoice]);

  if (!open) {
    return null;
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={(event) => {
        if (event.target === event.currentTarget && !busy) {
          onClose();
        }
      }}
    >
      <div
        className="bg-[var(--panel-glass)] border border-[var(--panel-border)] rounded-xl w-full max-w-2xl mx-4 max-h-[90vh] overflow-y-auto"
        style={{
          backdropFilter: "blur(var(--blur-md))",
          WebkitBackdropFilter: "blur(var(--blur-md))",
          boxShadow: "var(--shadow-panel)",
        }}
      >
        <div className="flex items-center justify-between p-4 border-b border-[var(--line)]">
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--text)]">Bulk Edit Services</h2>
            <p className="text-xs text-[var(--muted)] mt-0.5">
              Applies to {affectedCount} currently filtered service{affectedCount === 1 ? "" : "s"}.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="p-1 rounded hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
          >
            <X size={16} className="text-[var(--muted)]" />
          </button>
        </div>

        <div className="p-4 space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              Category
            </label>
            <select
              value={categoryChoice}
              onChange={(event) => setCategoryChoice(event.target.value)}
              disabled={busy}
              className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] cursor-pointer disabled:opacity-60"
            >
              <option value={noChangeValue}>No change</option>
              {CATEGORIES.map((category) => (
                <option key={category} value={category}>
                  {category}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              Visibility
            </label>
            <select
              value={visibilityChoice}
              onChange={(event) => setVisibilityChoice(event.target.value)}
              disabled={busy}
              className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] cursor-pointer disabled:opacity-60"
            >
              <option value={noChangeValue}>No change</option>
              <option value="hide">Hide</option>
              <option value="unhide">Unhide</option>
            </select>
          </div>

          <div className="space-y-2">
            <label className="block text-xs font-medium text-[var(--muted)]">Icon</label>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setIconMode("no_change")}
                disabled={busy}
                className={`h-7 px-2.5 rounded border text-xs font-medium transition-colors cursor-pointer ${
                  iconMode === "no_change"
                    ? "border-[var(--accent)] bg-[var(--accent)] text-[var(--accent-contrast)]"
                    : "border-[var(--line)] text-[var(--text)] hover:bg-[var(--hover)]"
                } disabled:opacity-60 disabled:cursor-not-allowed`}
              >
                No change
              </button>
              <button
                type="button"
                onClick={() => setIconMode("set")}
                disabled={busy}
                className={`h-7 px-2.5 rounded border text-xs font-medium transition-colors cursor-pointer ${
                  iconMode === "set"
                    ? "border-[var(--accent)] bg-[var(--accent)] text-[var(--accent-contrast)]"
                    : "border-[var(--line)] text-[var(--text)] hover:bg-[var(--hover)]"
                } disabled:opacity-60 disabled:cursor-not-allowed`}
              >
                Set icon
              </button>
              <button
                type="button"
                onClick={() => setIconMode("clear")}
                disabled={busy}
                className={`h-7 px-2.5 rounded border text-xs font-medium transition-colors cursor-pointer ${
                  iconMode === "clear"
                    ? "border-[var(--accent)] bg-[var(--accent)] text-[var(--accent-contrast)]"
                    : "border-[var(--line)] text-[var(--text)] hover:bg-[var(--hover)]"
                } disabled:opacity-60 disabled:cursor-not-allowed`}
              >
                Clear icon
              </button>
            </div>

            {iconMode === "set" && (
              <div className="space-y-2">
                <div className="text-xs text-[var(--muted)] flex items-center gap-2">
                  <ServiceIcon iconKey={iconValue} size={16} />
                  Selected: {iconSelectionLabel || "None"}
                </div>
                {customIcons.length > 0 && (
                  <div>
                    <p className="mb-1 text-xs text-[var(--muted)]">Custom Library</p>
                    <div className="grid grid-cols-10 gap-1 max-h-[120px] overflow-y-auto p-1 border border-[var(--line)] rounded">
                      {customIcons.map((icon) => (
                        <button
                          key={icon.id}
                          type="button"
                          onClick={() => setIconValue(icon.data_url)}
                          title={icon.name}
                          disabled={busy}
                          className={`w-8 h-8 rounded flex items-center justify-center transition-colors cursor-pointer ${
                            iconValue === icon.data_url
                              ? "bg-[var(--accent)]/20 border border-[var(--accent)]"
                              : "hover:bg-[var(--hover)] border border-transparent"
                          } disabled:opacity-60 disabled:cursor-not-allowed`}
                        >
                          <ServiceIcon iconKey={icon.data_url} size={18} />
                        </button>
                      ))}
                    </div>
                  </div>
                )}
                <IconPicker
                  selectedIcon={iconValue}
                  onSelect={setIconValue}
                  icons={icons}
                />
              </div>
            )}
          </div>

          {error && <p className="text-[12px] text-[var(--bad)]">{error}</p>}
        </div>

        <div className="flex items-center justify-end gap-2 p-4 border-t border-[var(--line)]">
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="h-7 px-3 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleApply()}
            disabled={busy}
            className="h-7 px-4 rounded bg-[var(--accent)] text-[var(--accent-contrast)] text-xs font-semibold hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {busy ? "Applying..." : "Apply to Filtered"}
          </button>
        </div>
      </div>
    </div>
  );
}

