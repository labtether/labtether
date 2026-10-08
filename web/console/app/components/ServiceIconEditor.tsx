"use client";

import { type ChangeEvent } from "react";
import { Pencil } from "lucide-react";
import { IconPicker } from "./IconPicker";
import { ServiceIcon } from "./ServiceIcon";
import type { ServiceCustomIcon } from "../hooks/useWebServices";
import { CUSTOM_ICON_ACCEPT, formatByteSize, MAX_CUSTOM_ICON_UPLOAD_BYTES } from "./serviceIconUpload";

export type ServiceIconEditorProps = {
  icons: string[];
  customIcons: ServiceCustomIcon[];
  iconKey: string;
  setIconKey: (value: string) => void;
  iconSelectionLabel: string;
  hasCustomIconSource: boolean;
  saving: boolean;
  removingURL: string | null;
  uploadingIcon: boolean;
  deletingCustomIconID: string | null;
  renamingCustomIconID: string | null;
  onDeleteCustomIcon: ((id: string) => Promise<void>) | undefined;
  onRenameCustomIcon: ((id: string, name: string) => Promise<ServiceCustomIcon>) | undefined;
  handleDeleteCustomIcon: (icon: ServiceCustomIcon) => Promise<void>;
  handleRenameCustomIcon: (icon: ServiceCustomIcon) => Promise<void>;
  handleUploadIcon: (event: ChangeEvent<HTMLInputElement>) => Promise<void>;
};

export function ServiceIconEditor({
  icons,
  customIcons,
  iconKey,
  setIconKey,
  iconSelectionLabel,
  hasCustomIconSource,
  saving,
  removingURL,
  uploadingIcon,
  deletingCustomIconID,
  renamingCustomIconID,
  onDeleteCustomIcon,
  onRenameCustomIcon,
  handleDeleteCustomIcon,
  handleRenameCustomIcon,
  handleUploadIcon,
}: ServiceIconEditorProps) {
  return (
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
  );
}
