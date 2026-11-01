"use client";

import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { Button } from "./ui/Button";
import { IconPicker } from "./IconPicker";
import { describeServiceIconSelection, ServiceIcon } from "./ServiceIcon";
import { SERVICE_CATEGORIES, ServiceDialogFrame } from "./ServiceEditModal";
import type { ServiceCustomIcon } from "../hooks/useWebServices";
import { readableServiceError } from "./serviceIconUpload";

const NO_CHANGE = "__no_change__";

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

/** One change (category, Services visibility, icon) for many services at once. Fields left at No change stay as they are. */
export function ServiceBulkEditModal({ open, affectedCount, icons, customIcons, busy = false, onApply, onClose }: ServiceBulkEditModalProps) {
  const t = useTranslations("services.bulkEdit");
  const [category, setCategory] = useState(NO_CHANGE);
  const [visibility, setVisibility] = useState(NO_CHANGE);
  const [iconMode, setIconMode] = useState<ServiceBulkEditChanges["iconMode"]>("no_change");
  const [iconValue, setIconValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const iconLabel = useMemo(() => describeServiceIconSelection(iconValue), [iconValue]);

  if (!open) return null;

  const apply = async () => {
    const changes: ServiceBulkEditChanges = { iconMode };
    if (category !== NO_CHANGE) changes.category = category;
    if (visibility !== NO_CHANGE) changes.hidden = visibility === "hide";
    if (iconMode === "set") {
      if (!iconValue.trim()) { setError(t("pickIcon")); return; }
      changes.iconValue = iconValue.trim();
    }
    if (changes.category === undefined && changes.hidden === undefined && iconMode === "no_change") {
      setError(t("nothing"));
      return;
    }
    setError(null);
    try {
      await onApply(changes);
      onClose();
    } catch (err) {
      setError(readableServiceError(err, t("failed")));
    }
  };

  return (
    <ServiceDialogFrame
      title={t("title", { count: affectedCount })}
      busy={busy}
      wide={iconMode === "set"}
      onClose={onClose}
      footer={
        <>
          <span className="lt-sp" />
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("cancel")}</Button>
          <Button variant="primary" onClick={() => void apply()} disabled={busy}>{busy ? t("applying") : t("apply", { count: affectedCount })}</Button>
        </>
      }
    >
      <div className="sv-ed-g">
        <div className="sv-field">
          <label htmlFor="sv-bulk-cat">{t("category")}</label>
          <select id="sv-bulk-cat" className="sv-input" value={category} onChange={(e) => setCategory(e.target.value)} disabled={busy}>
            <option value={NO_CHANGE}>{t("noChange")}</option>
            {SERVICE_CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </div>
        <div className="sv-field">
          <label htmlFor="sv-bulk-vis">{t("visibility")}</label>
          <select id="sv-bulk-vis" className="sv-input" value={visibility} onChange={(e) => setVisibility(e.target.value)} disabled={busy}>
            <option value={NO_CHANGE}>{t("noChange")}</option>
            <option value="hide">{t("hide")}</option>
            <option value="unhide">{t("show")}</option>
          </select>
        </div>
      </div>
      <div className="sv-field">
        <span className="sv-lbl">{t("icon")}</span>
        <div className="sv-seg" role="group" aria-label={t("icon")}>
          {(["no_change", "set", "clear"] as const).map((mode) => (
            <button key={mode} type="button" aria-pressed={iconMode === mode} onClick={() => setIconMode(mode)} disabled={busy}>{t(`iconModes.${mode}`)}</button>
          ))}
        </div>
      </div>
      {iconMode === "set" && (
        <div className="sv-well">
          <p className="sv-check"><ServiceIcon iconKey={iconValue} size={16} />{iconLabel || t("noIcon")}</p>
          {customIcons.length > 0 && (
            <div className="sv-icgrid">
              {customIcons.map((icon) => (
                <button key={icon.id} type="button" className={`sv-ictile${iconValue === icon.data_url ? " on" : ""}`} onClick={() => setIconValue(icon.data_url)}
                  disabled={busy} aria-pressed={iconValue === icon.data_url} aria-label={icon.name} title={icon.name}>
                  <ServiceIcon iconKey={icon.data_url} size={20} />
                </button>
              ))}
            </div>
          )}
          <IconPicker selectedIcon={iconValue} onSelect={setIconValue} icons={icons} />
        </div>
      )}
      {error && <p className="sv-err" role="alert">{error}</p>}
    </ServiceDialogFrame>
  );
}
