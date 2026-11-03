"use client";

import * as Dialog from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { Button } from "./ui/Button";
import { describeServiceIconSelection, ServiceIcon, serviceIconKey } from "./ServiceIcon";
import type { ServiceCustomIcon, WebServiceAltURL, WebService, WebServiceOverrideInput } from "../hooks/useWebServices";
import { ServiceIconEditor, useCustomIconActions } from "./ServiceIconEditor";
import { TagsEditor } from "./ServiceTagsEditor";
import { readableServiceError } from "./serviceIconUpload";

export const SERVICE_CATEGORIES = [
  "Media", "Downloads", "Gaming", "Networking", "Monitoring", "Home Automation", "Management",
  "Development", "Storage", "Databases", "Security", "Productivity", "Other",
];

/** The 2.0 dialog frame the service dialogs share: scrim, a raised sheet, the title and a close button. */
export function ServiceDialogFrame({ title, busy = false, wide = false, onClose, children, footer }: {
  title: string;
  busy?: boolean;
  wide?: boolean;
  onClose: () => void;
  children: ReactNode;
  footer: ReactNode;
}) {
  const t = useTranslations("services.edit");
  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open && !busy) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="sv-scrim" />
        <Dialog.Content
          className={`sv-dlg${wide ? " wide" : ""}`}
          aria-describedby={undefined}
          onOpenAutoFocus={(event) => {
            // Start in the first field, not on the close button.
            const field = (event.currentTarget as HTMLElement).querySelector<HTMLElement>(".sv-dlg-b input, .sv-dlg-b select");
            if (field) {
              event.preventDefault();
              field.focus();
            }
          }}
        >
          <div className="sv-dlg-h">
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Close className="sv-more sv-x" aria-label={t("close")} disabled={busy}><X aria-hidden="true" strokeWidth={2} /></Dialog.Close>
          </div>
          <div className="sv-dlg-b">{children}</div>
          <div className="sv-dlg-f">{footer}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

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

/**
 * Edit what LabTether found for a service: name, URL, category, icon, tags, other addresses and whether it shows in
 * Services. Reset to detected drops every change.
 */
export function ServiceEditModal({
  service, icons, customIcons = [], onSave, onReset, onCreateCustomIcon, onDeleteCustomIcon, onRenameCustomIcon, onAddAltURL, onRemoveAltURL, onClose,
}: ServiceEditModalProps) {
  const t = useTranslations("services.edit");
  const [name, setName] = useState(service.name);
  const [category, setCategory] = useState(service.category || "Other");
  const [iconKey, setIconKey] = useState(service.icon_key || "");
  const [hidden, setHidden] = useState(service.metadata?.hidden === "true");
  const [url, setURL] = useState(service.url);
  const [tags, setTags] = useState(service.metadata?.user_tags ?? "");
  const [picking, setPicking] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const report = useCallback((err: string | null, msg: string | null = null) => { setError(err); setMessage(msg); }, []);
  const iconActions = useCustomIconActions({ iconKey, setIconKey, report, onCreateCustomIcon, onDeleteCustomIcon, onRenameCustomIcon });
  const busy = saving || iconActions.busy;
  const iconLabel = useMemo(() => describeServiceIconSelection(iconKey), [iconKey]);

  const save = async () => {
    if (!name.trim()) { report(t("nameMissing")); return; }
    setSaving(true);
    report(null);
    try {
      await onSave({
        host_asset_id: service.host_asset_id,
        service_id: service.id,
        name_override: name !== service.name ? name.trim() : undefined,
        category_override: category !== service.category ? category : undefined,
        url_override: url !== service.url ? url.trim() : undefined,
        icon_key_override: iconKey !== service.icon_key ? iconKey : undefined,
        tags_override: tags.trim() || undefined,
        hidden,
      });
      onClose();
    } catch (err) {
      report(readableServiceError(err, t("saveFailed")));
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    setSaving(true);
    report(null);
    try {
      await onReset();
      onClose();
    } catch (err) {
      report(readableServiceError(err, t("resetFailed")));
    } finally {
      setSaving(false);
    }
  };

  return (
    <ServiceDialogFrame
      title={t("title", { name: service.name })}
      busy={busy}
      wide
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={() => void reset()} disabled={busy}>{t("reset")}</Button>
          <span className="lt-sp" />
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("cancel")}</Button>
          <Button variant="primary" onClick={() => void save()} disabled={busy}>{saving ? t("saving") : t("save")}</Button>
        </>
      }
    >
      <div className="sv-ed-g">
        <div className="sv-field">
          <label htmlFor="sv-ed-name">{t("name")}</label>
          <input id="sv-ed-name" className="sv-input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" />
        </div>
        <div className="sv-field">
          <label htmlFor="sv-ed-url">{t("url")}</label>
          <input id="sv-ed-url" className="sv-input mono" value={url} onChange={(e) => setURL(e.target.value)} placeholder="https://" autoComplete="off" />
        </div>
        <div className="sv-field">
          <label htmlFor="sv-ed-cat">{t("category")}</label>
          <select id="sv-ed-cat" className="sv-input" value={category} onChange={(e) => setCategory(e.target.value)}>
            {SERVICE_CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </div>
        <div className="sv-field">
          <span className="sv-lbl">{t("icon")}</span>
          <button type="button" className="sv-input sv-icbtn" onClick={() => setPicking((on) => !on)} aria-expanded={picking}>
            <ServiceIcon iconKey={iconKey || serviceIconKey(service)} size={16} />
            <span>{iconLabel || t("iconFound")}</span>
            <span className="sv-hint">{picking ? t("iconDone") : t("iconChange")}</span>
          </button>
        </div>
      </div>
      {picking && (
        <ServiceIconEditor icons={icons} customIcons={customIcons} iconKey={iconKey} setIconKey={setIconKey} disabled={busy} actions={iconActions} />
      )}
      <div className="sv-field">
        <span className="sv-lbl">{t("tags")}</span>
        <TagsEditor value={tags} onChange={setTags} />
      </div>
      <AltURLs service={service} busy={busy} report={report} onAdd={onAddAltURL} onRemove={onRemoveAltURL} />
      <label className="sv-check">
        <button type="button" role="switch" aria-checked={hidden} className="sv-switch" onClick={() => setHidden((on) => !on)} disabled={busy}><i /></button>
        {t("hide")}
      </label>
      {error && <p className="sv-err" role="alert">{error}</p>}
      {message && <p className="sv-hint" role="status">{message}</p>}
    </ServiceDialogFrame>
  );
}

/** Other addresses of the service: the list from the hub, remove one, add one. */
function AltURLs({ service, busy, report, onAdd, onRemove }: {
  service: WebService;
  busy: boolean;
  report: (err: string | null, msg?: string | null) => void;
  onAdd?: (webServiceID: string, url: string) => Promise<void>;
  onRemove?: (altURLID: string) => Promise<void>;
}) {
  const t = useTranslations("services.edit");
  const [list, setList] = useState<WebServiceAltURL[]>(service.alt_urls ?? []);
  const [draft, setDraft] = useState("");
  const [working, setWorking] = useState<string | null>(null);

  const reload = useCallback(async () => {
    const response = await fetch(`/api/services/web/alt-urls?web_service_id=${encodeURIComponent(service.url)}`, { cache: "no-store" });
    const payload = await response.json().catch(() => null) as { alt_urls?: WebServiceAltURL[]; error?: string } | null;
    if (!response.ok) throw new Error(payload?.error?.trim() || t("altLoadFailed"));
    setList(Array.isArray(payload?.alt_urls) ? payload.alt_urls : []);
  }, [service.url, t]);

  useEffect(() => {
    let live = true;
    void reload().catch((err) => { if (live) report(readableServiceError(err, t("altLoadFailed"))); });
    return () => { live = false; };
  }, [reload, report, t]);

  const run = async (key: string, work: () => Promise<void>, done: string) => {
    setWorking(key);
    report(null);
    try {
      await work();
      await reload();
      report(null, done);
      return true;
    } catch (err) {
      report(readableServiceError(err, t("altFailed")));
      return false;
    } finally {
      setWorking(null);
    }
  };
  const add = async () => {
    const value = draft.trim();
    if (!onAdd || !value) return;
    if (await run("add", () => onAdd(service.url, value), t("altAdded"))) setDraft("");
  };

  if (!onAdd && list.length === 0) return null;
  return (
    <div className="sv-field">
      <span className="sv-lbl">{t("altUrls")}</span>
      {list.length > 0 && (
        <ul className="sv-alts">
          {list.map((alt) => (
            <li key={alt.id}>
              <span className="mono">{alt.url}</span>
              <span className="sv-hint">{t(`altSource.${alt.source === "suggestion_accepted" ? "accepted" : alt.source === "manual" ? "manual" : "found"}`)}</span>
              {onRemove && (
                <button type="button" className="sv-more sv-x" disabled={busy || working !== null} aria-label={t("altRemove", { url: alt.url })}
                  onClick={() => void run(alt.id, () => onRemove(alt.id), t("altRemoved"))}>
                  <X aria-hidden="true" strokeWidth={2} />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {onAdd && (
        <div className="sv-row-in">
          <input className="sv-input mono" value={draft} placeholder="https://" aria-label={t("altAdd")} onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && draft.trim()) { e.preventDefault(); void add(); } }} />
          <Button onClick={() => void add()} disabled={!draft.trim() || busy || working !== null}>
            {t("add")}
          </Button>
        </div>
      )}
    </div>
  );
}
