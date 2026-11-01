"use client";

import { Pencil, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useState, type ChangeEvent } from "react";
import { IconPicker } from "./IconPicker";
import { isCustomServiceIconSource, ServiceIcon } from "./ServiceIcon";
import type { ServiceCustomIcon } from "../hooks/useWebServices";
import {
  CUSTOM_ICON_ACCEPT,
  CUSTOM_ICON_UPLOAD_TYPES,
  formatByteSize,
  MAX_CUSTOM_ICON_UPLOAD_BYTES,
  normalizeCustomIconName,
  readFileAsDataURL,
  readableServiceError,
} from "./serviceIconUpload";

type IconActionArgs = {
  iconKey: string;
  setIconKey: (value: string) => void;
  report: (error: string | null, message?: string | null) => void;
  onCreateCustomIcon?: (input: { name: string; data_url: string }) => Promise<ServiceCustomIcon>;
  onDeleteCustomIcon?: (id: string) => Promise<void>;
  onRenameCustomIcon?: (id: string, name: string) => Promise<ServiceCustomIcon>;
};

/** Upload an icon to the library (and pick it), delete one, rename one. Results go to the dialog's message line. */
export function useCustomIconActions({ iconKey, setIconKey, report, onCreateCustomIcon, onDeleteCustomIcon, onRenameCustomIcon }: IconActionArgs) {
  const t = useTranslations("services.icons");
  const [working, setWorking] = useState<string | null>(null);

  const upload = useCallback(async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!file) return;
    if (!CUSTOM_ICON_UPLOAD_TYPES.has(file.type.toLowerCase().trim())) { report(t("badType")); return; }
    if (file.size > MAX_CUSTOM_ICON_UPLOAD_BYTES) { report(t("tooLarge", { max: formatByteSize(MAX_CUSTOM_ICON_UPLOAD_BYTES) })); return; }
    setWorking("upload");
    report(null);
    try {
      const data = await readFileAsDataURL(file);
      if (!isCustomServiceIconSource(data)) throw new Error(t("unreadable"));
      if (onCreateCustomIcon) {
        const created = await onCreateCustomIcon({ name: normalizeCustomIconName(file.name), data_url: data });
        setIconKey(created.data_url);
        report(null, t("uploaded", { name: created.name }));
      } else {
        setIconKey(data);
        report(null, t("uploaded", { name: file.name }));
      }
    } catch (err) {
      report(readableServiceError(err, t("unreadable")));
    } finally {
      setWorking(null);
    }
  }, [onCreateCustomIcon, report, setIconKey, t]);

  const remove = useCallback(async (icon: ServiceCustomIcon) => {
    if (!onDeleteCustomIcon || !window.confirm(t("deleteAsk", { name: icon.name }))) return;
    setWorking(icon.id);
    report(null);
    try {
      await onDeleteCustomIcon(icon.id);
      if (iconKey === icon.data_url) setIconKey("");
      report(null, t("deleted", { name: icon.name }));
    } catch (err) {
      report(readableServiceError(err, t("deleteFailed")));
    } finally {
      setWorking(null);
    }
  }, [iconKey, onDeleteCustomIcon, report, setIconKey, t]);

  const rename = useCallback(async (icon: ServiceCustomIcon) => {
    if (!onRenameCustomIcon) return;
    const next = (window.prompt(t("renameAsk"), icon.name) ?? "").trim();
    if (!next || next === icon.name) return;
    setWorking(icon.id);
    report(null);
    try {
      const renamed = await onRenameCustomIcon(icon.id, next);
      report(null, t("renamed", { name: renamed.name }));
    } catch (err) {
      report(readableServiceError(err, t("renameFailed")));
    } finally {
      setWorking(null);
    }
  }, [onRenameCustomIcon, report, t]);

  return {
    busy: working !== null,
    uploading: working === "upload",
    working,
    upload,
    remove: onDeleteCustomIcon ? remove : undefined,
    rename: onRenameCustomIcon ? rename : undefined,
  };
}

export type ServiceIconEditorProps = {
  icons: string[];
  customIcons: ServiceCustomIcon[];
  iconKey: string;
  setIconKey: (value: string) => void;
  disabled: boolean;
  actions: ReturnType<typeof useCustomIconActions>;
};

/** The icon chooser in a sunk well: the uploaded library, the built-in set with search, and Upload. */
export function ServiceIconEditor({ icons, customIcons, iconKey, setIconKey, disabled, actions }: ServiceIconEditorProps) {
  const t = useTranslations("services.icons");
  return (
    <div className="sv-well">
      {customIcons.length > 0 && (
        <div className="sv-field">
          <span className="sv-lbl">{t("library")}</span>
          <div className="sv-icgrid">
            {customIcons.map((icon) => (
              <span key={icon.id} className="sv-icl">
                <button type="button" className={`sv-ictile${iconKey === icon.data_url ? " on" : ""}`} onClick={() => setIconKey(icon.data_url)}
                  disabled={disabled} aria-pressed={iconKey === icon.data_url} aria-label={icon.name} title={icon.name}>
                  <ServiceIcon iconKey={icon.data_url} size={20} />
                </button>
                {actions.rename && (
                  <button type="button" className="sv-icl-a l" onClick={() => void actions.rename?.(icon)} disabled={disabled} aria-label={t("rename", { name: icon.name })}>
                    {actions.working === icon.id ? "…" : <Pencil aria-hidden="true" />}
                  </button>
                )}
                {actions.remove && (
                  <button type="button" className="sv-icl-a r" onClick={() => void actions.remove?.(icon)} disabled={disabled} aria-label={t("delete", { name: icon.name })}>
                    {actions.working === icon.id ? "…" : <X aria-hidden="true" />}
                  </button>
                )}
              </span>
            ))}
          </div>
        </div>
      )}
      <IconPicker selectedIcon={iconKey} onSelect={setIconKey} icons={icons} />
      <div className="sv-row-in">
        <label className={`sv-chip${disabled ? " off" : ""}`}>
          {actions.uploading ? t("uploading") : t("upload")}
          <input type="file" accept={CUSTOM_ICON_ACCEPT} onChange={(event) => void actions.upload(event)} className="sr-only" disabled={disabled} />
        </label>
        {isCustomServiceIconSource(iconKey) && (
          <button type="button" className="sv-chip" onClick={() => setIconKey("")} disabled={disabled}>{t("clearCustom")}</button>
        )}
        <span className="sv-hint">{t("limits", { max: formatByteSize(MAX_CUSTOM_ICON_UPLOAD_BYTES) })}</span>
      </div>
    </div>
  );
}
