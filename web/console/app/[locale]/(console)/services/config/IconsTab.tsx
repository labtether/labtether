"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { ImageIcon, Pencil, Trash2, Upload } from "lucide-react";
import { Button } from "../../../../components/ui/Button";
import { EmptyState } from "../../../../components/ui/EmptyState";
import {
  useWebServices,
  type ServiceCustomIcon,
} from "../../../../hooks/useWebServices";
import { useServiceIconLibrary } from "../useServiceIconLibrary";

const MAX_FILE_SIZE_BYTES = 512 * 1024;
const ACCEPTED_MIME_TYPES =
  "image/png,image/jpeg,image/webp,image/gif,image/svg+xml";

export default function IconsTab() {
  const t = useTranslations("services");

  const {
    services,
    listCustomServiceIcons,
    createCustomServiceIcon,
    deleteCustomServiceIcon,
    renameCustomServiceIcon,
  } = useWebServices({ detailLevel: "compact" });

  const { customIcons, createCustomIcon, deleteCustomIcon, renameCustomIcon } =
    useServiceIconLibrary({
      listCustomServiceIcons,
      createCustomServiceIcon,
      deleteCustomServiceIcon,
      renameCustomServiceIcon,
    });

  const fileInputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);

  // Track which icon is being renamed and its in-progress name value
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameLoading, setRenameLoading] = useState(false);

  const [deletingId, setDeletingId] = useState<string | null>(null);

  const usageCount = useCallback(
    (icon: ServiceCustomIcon): number => {
      return services.filter((s) => s.icon_key === icon.data_url).length;
    },
    [services]
  );

  const handleUploadClick = useCallback(() => {
    setUploadError(null);
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;

      // Reset input so the same file can be re-selected after an error
      e.target.value = "";

      if (file.size > MAX_FILE_SIZE_BYTES) {
        setUploadError(t("config.icons.fileTooLarge"));
        return;
      }

      setUploadError(null);
      setUploading(true);
      try {
        const dataUrl = await readFileAsDataURL(file);
        await createCustomIcon({ name: file.name, data_url: dataUrl });
      } catch (err) {
        setUploadError(err instanceof Error ? err.message : t("config.icons.uploadFailed"));
      } finally {
        setUploading(false);
      }
    },
    [createCustomIcon, t]
  );

  const startRename = useCallback((icon: ServiceCustomIcon) => {
    setRenamingId(icon.id);
    setRenameValue(icon.name);
  }, []);

  const cancelRename = useCallback(() => {
    setRenamingId(null);
    setRenameValue("");
  }, []);

  const confirmRename = useCallback(
    async (id: string) => {
      const trimmed = renameValue.trim();
      if (!trimmed) return;
      setRenameLoading(true);
      try {
        await renameCustomIcon(id, trimmed);
        setRenamingId(null);
        setRenameValue("");
      } catch {
        // Error surfaced by hook
      } finally {
        setRenameLoading(false);
      }
    },
    [renameCustomIcon, renameValue]
  );

  const handleDelete = useCallback(
    async (icon: ServiceCustomIcon) => {
      const count = usageCount(icon);
      const message =
        count > 0
          ? t("config.icons.deleteInUse", { count })
          : t("config.icons.deleteConfirm");
      if (!window.confirm(message)) return;

      setDeletingId(icon.id);
      try {
        await deleteCustomIcon(icon.id);
      } catch {
        // Error surfaced by hook
      } finally {
        setDeletingId(null);
      }
    },
    [deleteCustomIcon, usageCount, t]
  );

  return (
    <div className="flex flex-col gap-4">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="secondary"
          size="sm"
          loading={uploading}
          onClick={handleUploadClick}
        >
          <Upload className="h-3.5 w-3.5" />
          {t("config.icons.upload")}
        </Button>
        <span className="text-xs text-[var(--muted)]">
          {t("config.icons.maxSize")}
        </span>
        <input
          ref={fileInputRef}
          type="file"
          accept={ACCEPTED_MIME_TYPES}
          className="hidden"
          onChange={handleFileChange}
        />
        {uploadError && (
          <span className="text-xs text-[var(--bad)]">{uploadError}</span>
        )}
      </div>

      {/* Grid or empty state */}
      {customIcons.length === 0 ? (
        <EmptyState
          icon={ImageIcon}
          title={t("config.tabs.icons")}
          description={t("config.icons.emptyState")}
        />
      ) : (
        <div className="grid gap-3 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {customIcons.map((icon) => {
            const count = usageCount(icon);
            const isRenaming = renamingId === icon.id;
            const isDeleting = deletingId === icon.id;

            return (
              <div
                key={icon.id}
                className="flex flex-col gap-2 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3 transition-colors hover:border-[var(--accent)]/40"
              >
                {/* Thumbnail */}
                <div className="flex items-center justify-center h-16 rounded-lg bg-[var(--bg)] overflow-hidden">
                  <img
                    src={icon.data_url}
                    alt={icon.name}
                    className="max-h-full max-w-full object-contain"
                    loading="lazy"
                  />
                </div>

                {/* Name: view or inline-edit */}
                {isRenaming ? (
                  <div className="flex items-center gap-1">
                    <input
                      autoFocus
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") void confirmRename(icon.id);
                        if (e.key === "Escape") cancelRename();
                      }}
                      className="min-w-0 flex-1 rounded border border-[var(--control-border)] bg-[var(--bg)] px-1.5 py-0.5 text-xs text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--control-focus-ring)]"
                      disabled={renameLoading}
                    />
                    <button
                      type="button"
                      onClick={() => void confirmRename(icon.id)}
                      disabled={renameLoading || !renameValue.trim()}
                      className="shrink-0 rounded px-1 py-0.5 text-[10px] font-semibold text-[var(--accent)] hover:bg-[var(--hover)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
                    >
                      OK
                    </button>
                    <button
                      type="button"
                      onClick={cancelRename}
                      disabled={renameLoading}
                      className="shrink-0 rounded px-1 py-0.5 text-[10px] text-[var(--muted)] hover:bg-[var(--hover)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
                    >
                      ✕
                    </button>
                  </div>
                ) : (
                  <p
                    className="truncate text-xs font-medium text-[var(--text)] cursor-pointer hover:text-[var(--accent)] transition-colors"
                    title={icon.name}
                    onClick={() => startRename(icon)}
                  >
                    {icon.name}
                  </p>
                )}

                {/* Usage badge */}
                <span
                  className={`text-[10px] font-medium ${
                    count > 0
                      ? "text-[var(--accent)]"
                      : "text-[var(--muted)]"
                  }`}
                >
                  {count > 0
                    ? t("config.icons.usedBy", { count })
                    : t("config.icons.notUsed")}
                </span>

                {/* Action buttons */}
                <div className="flex items-center gap-1 pt-0.5">
                  <button
                    type="button"
                    onClick={() => startRename(icon)}
                    disabled={isRenaming || isDeleting}
                    title={t("config.icons.rename")}
                    className="inline-flex items-center gap-1 rounded px-1.5 py-1 text-[10px] text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
                  >
                    <Pencil className="h-3 w-3" />
                    {t("config.icons.rename")}
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleDelete(icon)}
                    disabled={isDeleting || isRenaming}
                    title={t("config.icons.delete")}
                    className="inline-flex items-center gap-1 rounded px-1.5 py-1 text-[10px] text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
                  >
                    <Trash2 className="h-3 w-3" />
                    {t("config.icons.delete")}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") {
        resolve(reader.result);
      } else {
        reject(new Error("FileReader did not return a string"));
      }
    };
    reader.onerror = () => reject(reader.error ?? new Error("FileReader error"));
    reader.readAsDataURL(file);
  });
}
