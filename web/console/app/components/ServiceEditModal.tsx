"use client";

import { useState, useEffect, useCallback, useMemo, type ChangeEvent } from "react";
import { Pencil, Plus, X } from "lucide-react";
import { IconPicker } from "./IconPicker";
import {
  describeServiceIconSelection,
  isCustomServiceIconSource,
  ServiceIcon,
} from "./ServiceIcon";
import type {
  ServiceCustomIcon,
  WebServiceAltURL,
  WebService,
  WebServiceOverrideInput,
} from "../hooks/useWebServices";

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

const MAX_CUSTOM_ICON_UPLOAD_BYTES = 512 * 1024;
const CUSTOM_ICON_UPLOAD_TYPES = new Set([
  "image/png",
  "image/jpeg",
  "image/webp",
  "image/gif",
  "image/svg+xml",
]);
const CUSTOM_ICON_ACCEPT = Array.from(CUSTOM_ICON_UPLOAD_TYPES).join(",");

interface ServiceEditModalProps {
  service: WebService;
  icons: string[];
  customIcons?: ServiceCustomIcon[];
  onSave: (override: WebServiceOverrideInput) => Promise<void>;
  onReset: () => Promise<void>;
  onCreateCustomIcon?: (input: { name: string; data_url: string }) => Promise<ServiceCustomIcon>;
  onDeleteCustomIcon?: (id: string) => Promise<void>;
  onRenameCustomIcon?: (id: string, name: string) => Promise<ServiceCustomIcon>;
  onAddAltURL?: (webServiceID: string, url: string) => Promise<void>;
  onRemoveAltURL?: (altURLID: string) => Promise<void>;
  onClose: () => void;
}

export function ServiceEditModal({
  service,
  icons,
  customIcons = [],
  onSave,
  onReset,
  onCreateCustomIcon,
  onDeleteCustomIcon,
  onRenameCustomIcon,
  onAddAltURL,
  onRemoveAltURL,
  onClose,
}: ServiceEditModalProps) {
  const [name, setName] = useState(service.name);
  const [category, setCategory] = useState(service.category || "Other");
  const [iconKey, setIconKey] = useState(service.icon_key || "");
  const [hidden, setHidden] = useState(service.metadata?.hidden === "true");
  const [urlOverride, setUrlOverride] = useState(service.url);
  const [tagsInput, setTagsInput] = useState(service.metadata?.user_tags ?? "");
  const [saving, setSaving] = useState(false);
  const [removingURL, setRemovingURL] = useState<string | null>(null);
  const [newAltURL, setNewAltURL] = useState("");
  const [altURLs, setAltURLs] = useState<WebServiceAltURL[]>(service.alt_urls ?? []);
  const [uploadingIcon, setUploadingIcon] = useState(false);
  const [deletingCustomIconID, setDeletingCustomIconID] = useState<string | null>(null);
  const [renamingCustomIconID, setRenamingCustomIconID] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const iconSelectionLabel = useMemo(() => describeServiceIconSelection(iconKey), [iconKey]);
  const hasCustomIconSource = useMemo(() => isCustomServiceIconSource(iconKey), [iconKey]);

  useEffect(() => {
    setName(service.name);
    setCategory(service.category || "Other");
    setIconKey(service.icon_key || "");
    setHidden(service.metadata?.hidden === "true");
    setUrlOverride(service.url);
    setTagsInput(service.metadata?.user_tags ?? "");
    setRemovingURL(null);
    setNewAltURL("");
    setAltURLs(service.alt_urls ?? []);
    setUploadingIcon(false);
    setDeletingCustomIconID(null);
    setRenamingCustomIconID(null);
    setError(null);
    setMessage(null);
  }, [service]);

  const reloadAltURLs = useCallback(async () => {
    const response = await fetch(`/api/services/web/alt-urls?web_service_id=${encodeURIComponent(service.url)}`, {
      cache: "no-store",
    });
    const payload = await response.json().catch(() => null) as { alt_urls?: WebServiceAltURL[]; error?: string } | null;
    if (!response.ok) {
      throw new Error(payload?.error ?? `Failed to load alt URLs (HTTP ${response.status})`);
    }
    setAltURLs(Array.isArray(payload?.alt_urls) ? payload.alt_urls : []);
  }, [service.url]);

  useEffect(() => {
    let cancelled = false;
    void reloadAltURLs().catch((err) => {
      if (cancelled) {
        return;
      }
      setError(err instanceof Error ? err.message : "Failed to load alternative URLs");
    });
    return () => {
      cancelled = true;
    };
  }, [reloadAltURLs]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await onSave({
        host_asset_id: service.host_asset_id,
        service_id: service.id,
        name_override: name !== service.name ? name : undefined,
        category_override: category !== service.category ? category : undefined,
        url_override: urlOverride !== service.url ? urlOverride : undefined,
        icon_key_override: iconKey !== service.icon_key ? iconKey : undefined,
        tags_override: tagsInput.trim() || undefined,
        hidden,
      });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  }, [service, name, category, urlOverride, iconKey, tagsInput, hidden, onSave, onClose]);

  const handleReset = useCallback(async () => {
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await onReset();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to reset");
    } finally {
      setSaving(false);
    }
  }, [onReset, onClose]);

  const handleAddAltURL = useCallback(async () => {
    if (!onAddAltURL || !newAltURL.trim()) return;
    setError(null);
    setMessage(null);
    try {
      await onAddAltURL(service.url, newAltURL.trim());
      await reloadAltURLs();
      setNewAltURL("");
      setMessage("Alternative URL added.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add URL");
    }
  }, [newAltURL, onAddAltURL, reloadAltURLs, service.url]);

  const handleRemoveAltURL = useCallback(async (altURLID: string) => {
    if (!onRemoveAltURL) return;
    setRemovingURL(altURLID);
    setError(null);
    setMessage(null);
    try {
      await onRemoveAltURL(altURLID);
      await reloadAltURLs();
      setMessage("Alternative URL removed.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove URL");
    } finally {
      setRemovingURL(null);
    }
  }, [onRemoveAltURL, reloadAltURLs]);

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  const handleUploadIcon = useCallback(async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!file) {
      return;
    }

    const type = file.type.toLowerCase().trim();
    if (!CUSTOM_ICON_UPLOAD_TYPES.has(type)) {
      setError("Icon must be PNG, JPEG, WEBP, GIF, or SVG.");
      setMessage(null);
      return;
    }
    if (file.size > MAX_CUSTOM_ICON_UPLOAD_BYTES) {
      setError(`Icon is too large. Maximum size is ${formatByteSize(MAX_CUSTOM_ICON_UPLOAD_BYTES)}.`);
      setMessage(null);
      return;
    }

    setError(null);
    setMessage(null);
    setUploadingIcon(true);
    try {
      const uploaded = await readFileAsDataURL(file);
      if (!isCustomServiceIconSource(uploaded)) {
        throw new Error("invalid icon data");
      }
      if (onCreateCustomIcon) {
        const created = await onCreateCustomIcon({
          name: normalizeCustomIconName(file.name),
          data_url: uploaded,
        });
        setIconKey(created.data_url);
        setMessage(`Uploaded "${created.name}" to icon library.`);
      } else {
        setIconKey(uploaded);
        setMessage(`Uploaded custom icon: ${file.name}`);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to read icon file.");
      setMessage(null);
    } finally {
      setUploadingIcon(false);
    }
  }, [onCreateCustomIcon]);

  const handleDeleteCustomIcon = useCallback(async (icon: ServiceCustomIcon) => {
    if (!onDeleteCustomIcon) {
      return;
    }
    const confirmed = window.confirm(`Delete custom icon "${icon.name}" from library?`);
    if (!confirmed) {
      return;
    }

    setDeletingCustomIconID(icon.id);
    setError(null);
    setMessage(null);
    try {
      await onDeleteCustomIcon(icon.id);
      if (iconKey === icon.data_url) {
        setIconKey("");
      }
      setMessage(`Deleted "${icon.name}" from icon library.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete icon");
    } finally {
      setDeletingCustomIconID(null);
    }
  }, [iconKey, onDeleteCustomIcon]);

  const handleRenameCustomIcon = useCallback(async (icon: ServiceCustomIcon) => {
    if (!onRenameCustomIcon) {
      return;
    }
    const proposed = (window.prompt("Icon name", icon.name) ?? "").trim();
    if (!proposed || proposed === icon.name) {
      return;
    }

    setRenamingCustomIconID(icon.id);
    setError(null);
    setMessage(null);
    try {
      const renamed = await onRenameCustomIcon(icon.id, proposed);
      setMessage(`Renamed icon to "${renamed.name}".`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to rename icon");
    } finally {
      setRenamingCustomIconID(null);
    }
  }, [onRenameCustomIcon]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className="bg-[var(--panel-glass)] border border-[var(--panel-border)] rounded-xl w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto"
        style={{
          backdropFilter: "blur(var(--blur-md))",
          WebkitBackdropFilter: "blur(var(--blur-md))",
          boxShadow: "var(--shadow-panel)",
        }}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-[var(--line)]">
          <div className="flex items-center gap-2.5">
            <ServiceIcon iconKey={iconKey || service.icon_key} size={24} />
            <h2 className="text-[15px] font-semibold text-[var(--text)]">
              Edit Service
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-[var(--hover)] transition-colors cursor-pointer"
          >
            <X size={16} className="text-[var(--muted)]" />
          </button>
        </div>

        {/* Body */}
        <div className="p-4 space-y-4">
          {/* Name */}
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              Display Name
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
            />
          </div>

          {/* URL Override */}
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              URL
            </label>
            <input
              type="text"
              value={urlOverride}
              onChange={(e) => setUrlOverride(e.target.value)}
              placeholder="https://..."
              className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] font-mono"
            />
            <p className="mt-0.5 text-[10px] text-[var(--muted)]">
              Override the discovered URL for this service.
            </p>
          </div>

          {/* Category */}
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              Category
            </label>
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] cursor-pointer"
            >
              {CATEGORIES.map((cat) => (
                <option key={cat} value={cat}>
                  {cat}
                </option>
              ))}
            </select>
          </div>

          {/* Icon */}
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              Icon {iconSelectionLabel && <span className="text-[var(--text)]">— {iconSelectionLabel}</span>}
            </label>
            {customIcons.length > 0 && (
              <div className="mb-2">
                <p className="mb-1 text-xs text-[var(--muted)]">Custom Library</p>
                <div className="grid grid-cols-8 gap-1 max-h-[120px] overflow-y-auto p-1 border border-[var(--line)] rounded">
                  {customIcons.map((icon) => (
                    <div key={icon.id} className="relative">
                      <button
                        type="button"
                        onClick={() => setIconKey(icon.data_url)}
                        title={icon.name}
                        disabled={
                          saving
                          || removingURL !== null
                          || uploadingIcon
                          || deletingCustomIconID !== null
                          || renamingCustomIconID !== null
                        }
                        className={`w-9 h-9 rounded flex items-center justify-center transition-colors duration-[var(--dur-fast)] cursor-pointer ${
                          iconKey === icon.data_url
                            ? "bg-[var(--accent)]/20 border border-[var(--accent)]"
                            : "hover:bg-[var(--hover)] border border-transparent"
                        } disabled:opacity-60 disabled:cursor-not-allowed`}
                      >
                        <ServiceIcon iconKey={icon.data_url} size={20} />
                      </button>
                      {onDeleteCustomIcon && (
                        <button
                          type="button"
                          onClick={() => void handleDeleteCustomIcon(icon)}
                          disabled={
                            saving
                            || removingURL !== null
                            || uploadingIcon
                            || deletingCustomIconID !== null
                            || renamingCustomIconID !== null
                          }
                          className="absolute -top-1 -right-1 w-4 h-4 rounded-full bg-[var(--panel)] border border-[var(--line)] text-[10px] text-[var(--muted)] hover:text-[var(--bad)] transition-colors cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
                          title={`Delete ${icon.name}`}
                          aria-label={`Delete ${icon.name}`}
                        >
                          {deletingCustomIconID === icon.id ? "…" : "×"}
                        </button>
                      )}
                      {onRenameCustomIcon && (
                        <button
                          type="button"
                          onClick={() => void handleRenameCustomIcon(icon)}
                          disabled={
                            saving
                            || removingURL !== null
                            || uploadingIcon
                            || deletingCustomIconID !== null
                            || renamingCustomIconID !== null
                          }
                          className="absolute -top-1 -left-1 w-4 h-4 rounded-full bg-[var(--panel)] border border-[var(--line)] text-[10px] text-[var(--muted)] hover:text-[var(--text)] transition-colors cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed inline-flex items-center justify-center"
                          title={`Rename ${icon.name}`}
                          aria-label={`Rename ${icon.name}`}
                        >
                          {renamingCustomIconID === icon.id ? "…" : <Pencil size={9} />}
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
            <IconPicker
              selectedIcon={iconKey}
              onSelect={setIconKey}
              icons={icons}
            />
            <div className="mt-2 flex items-center gap-2">
              <label className="h-7 px-3 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer inline-flex items-center">
                {uploadingIcon ? "Uploading..." : "Upload icon"}
                <input
                  type="file"
                  accept={CUSTOM_ICON_ACCEPT}
                  onChange={(event) => void handleUploadIcon(event)}
                  className="hidden"
                  disabled={
                    saving
                    || removingURL !== null
                    || uploadingIcon
                    || deletingCustomIconID !== null
                    || renamingCustomIconID !== null
                  }
                />
              </label>
              {hasCustomIconSource && (
                <button
                  type="button"
                  onClick={() => setIconKey("")}
                  disabled={
                    saving
                    || removingURL !== null
                    || uploadingIcon
                    || deletingCustomIconID !== null
                    || renamingCustomIconID !== null
                  }
                  className="h-7 px-3 rounded text-xs font-medium text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-50"
                >
                  Clear custom
                </button>
              )}
            </div>
            <p className="mt-1 text-xs text-[var(--muted)]">
              PNG, JPEG, WEBP, GIF, or SVG up to {formatByteSize(MAX_CUSTOM_ICON_UPLOAD_BYTES)}.
            </p>
          </div>

          {/* Hidden toggle */}
          <div className="flex items-center justify-between">
            <label className="text-[13px] text-[var(--text)]">
              Hidden
            </label>
            <button
              type="button"
              onClick={() => setHidden(!hidden)}
              disabled={removingURL !== null || uploadingIcon || deletingCustomIconID !== null || renamingCustomIconID !== null}
              className={`w-9 h-5 rounded-full transition-colors cursor-pointer ${
                hidden ? "bg-[var(--accent)]" : "bg-[var(--surface)] border border-[var(--line)]"
              } disabled:opacity-60 disabled:cursor-not-allowed`}
            >
              <div
                className={`w-3.5 h-3.5 rounded-full bg-white transition-transform ${
                  hidden ? "translate-x-[18px]" : "translate-x-[3px]"
                }`}
              />
            </button>
          </div>

          {/* Tags */}
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1">
              Tags
            </label>
            <TagsEditor value={tagsInput} onChange={setTagsInput} />
          </div>

          {(altURLs.length > 0 || onAddAltURL) && (
            <div className="space-y-2">
              <label className="block text-xs font-medium text-[var(--muted)]">
                Alternative URLs
              </label>
              {altURLs.length > 0 && (
                <div className="space-y-1.5">
                  {altURLs.map((alt) => (
                    <div
                      key={alt.id}
                      className="flex items-center gap-2 group"
                    >
                      <span className={`shrink-0 text-[10px] px-1.5 py-0.5 rounded font-mono ${
                        alt.source === "manual" ? "bg-[var(--accent)]/15 text-[var(--accent)]" :
                        alt.source === "suggestion_accepted" ? "bg-[var(--good)]/15 text-[var(--good)]" :
                        "bg-[var(--surface)] text-[var(--muted)]"
                      }`}>
                        {alt.source === "suggestion_accepted" ? "accepted" : alt.source}
                      </span>
                      <span className="text-[12px] text-[var(--text)] truncate flex-1" title={alt.url}>
                        {alt.url}
                      </span>
                      {onRemoveAltURL && (
                        <button
                          type="button"
                          onClick={() => void handleRemoveAltURL(alt.id)}
                          disabled={saving || removingURL !== null || uploadingIcon || deletingCustomIconID !== null || renamingCustomIconID !== null}
                          className="shrink-0 opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-[var(--hover)] text-[var(--muted)] hover:text-[var(--bad)] transition-[color,background-color,opacity] cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
                          title="Remove alternative URL"
                          aria-label={`Remove alternative URL ${alt.url}`}
                        >
                          {removingURL === alt.id ? (
                            <span className="block w-3 h-3 rounded-full border border-[var(--muted)] border-t-transparent animate-spin" />
                          ) : (
                            <X size={12} />
                          )}
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              )}
              {onAddAltURL && (
                <div className="flex gap-2 mt-1">
                  <input
                    type="text"
                    placeholder="Add alternative URL..."
                    value={newAltURL}
                    onChange={(e) => setNewAltURL(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        void handleAddAltURL();
                      }
                    }}
                    className="flex-1 h-7 px-2.5 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] placeholder-[var(--muted)] focus:outline-none focus:border-[var(--accent)] font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => void handleAddAltURL()}
                    disabled={!newAltURL.trim() || saving || removingURL !== null}
                    className="h-7 px-3 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
                  >
                    Add
                  </button>
                </div>
              )}
            </div>
          )}

          {error && (
            <p className="text-[12px] text-[var(--bad)]">{error}</p>
          )}
          {message && (
            <p className="text-[12px] text-[var(--muted)]">{message}</p>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between p-4 border-t border-[var(--line)]">
          <button
            type="button"
            onClick={handleReset}
            disabled={saving || removingURL !== null || uploadingIcon || deletingCustomIconID !== null || renamingCustomIconID !== null}
            className="h-7 px-3 rounded text-xs font-medium text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-50"
          >
            Reset to Auto
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={saving || removingURL !== null || uploadingIcon || deletingCustomIconID !== null || renamingCustomIconID !== null}
              className="h-7 px-3 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={saving || removingURL !== null || uploadingIcon || deletingCustomIconID !== null || renamingCustomIconID !== null}
              className="h-7 px-4 rounded bg-[var(--accent)] text-[var(--accent-contrast)] text-xs font-semibold hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50"
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function TagsEditor({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const tags = value
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
  const [draft, setDraft] = useState("");

  function addTag() {
    const trimmed = draft.trim().toLowerCase();
    if (!trimmed) return;
    if (tags.includes(trimmed)) {
      setDraft("");
      return;
    }
    onChange([...tags, trimmed].join(", "));
    setDraft("");
  }

  function removeTag(tag: string) {
    onChange(tags.filter((t) => t !== tag).join(", "));
  }

  return (
    <div>
      {tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-2">
          {tags.map((tag) => (
            <span
              key={tag}
              className="inline-flex items-center gap-1 h-5 px-2 rounded-full bg-[var(--accent)]/15 border border-[var(--accent)]/25 text-[10px] font-medium text-[var(--accent)]"
            >
              {tag}
              <button
                type="button"
                onClick={() => removeTag(tag)}
                className="hover:text-[var(--bad)] transition-colors cursor-pointer"
              >
                <X size={10} />
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="flex items-center gap-1.5">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addTag();
            }
          }}
          placeholder="Add tag..."
          className="flex-1 h-7 px-2.5 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
        />
        <button
          type="button"
          onClick={addTag}
          className="h-7 w-7 rounded border border-[var(--line)] hover:bg-[var(--hover)] transition-colors cursor-pointer inline-flex items-center justify-center"
        >
          <Plus size={12} className="text-[var(--muted)]" />
        </button>
      </div>
    </div>
  );
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result !== "string") {
        reject(new Error("invalid file"));
        return;
      }
      resolve(reader.result);
    };
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(file);
  });
}

function normalizeCustomIconName(filename: string): string {
  const trimmed = filename.trim();
  if (!trimmed) {
    return "Custom Icon";
  }
  const withoutExtension = trimmed.replace(/\.[^.]+$/, "");
  const normalized = withoutExtension.replace(/[_-]+/g, " ").trim();
  return normalized || "Custom Icon";
}

function formatByteSize(size: number): string {
  if (size >= 1024 * 1024) {
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${Math.round(size / 1024)} KB`;
}
