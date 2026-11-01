"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../../../components/ui/Button";
import type { ManualWebServiceInput } from "../../../hooks/useWebServices";
import { CATEGORY_ORDER } from "./servicesLayoutHelpers";
import { readableError } from "./servicesListModel";

type Errors = { name?: string; url?: string; form?: string };

function validURL(raw: string): boolean {
  try {
    const u = new URL(raw);
    return (u.protocol === "http:" || u.protocol === "https:") && u.hostname !== "";
  } catch {
    return false;
  }
}

/**
 * The Add service form, a sheet above the list: name, URL, the device it runs on (optional) and its category.
 * LabTether checks the URL like any found service.
 */
export function ServiceAddForm({
  devices,
  defaultDevice,
  onAdd,
  onClose,
}: {
  devices: Array<[string, string]>;
  defaultDevice: string;
  onAdd: (input: ManualWebServiceInput) => Promise<void>;
  onClose: () => void;
}) {
  const t = useTranslations("services.add");
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [device, setDevice] = useState(defaultDevice);
  const [category, setCategory] = useState("Other");
  const [errors, setErrors] = useState<Errors>({});
  const [saving, setSaving] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  const urlRef = useRef<HTMLInputElement>(null);

  useEffect(() => { nameRef.current?.focus(); }, []);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const next: Errors = {};
    if (!name.trim()) next.name = t("nameMissing");
    if (!url.trim()) next.url = t("urlMissing");
    else if (!validURL(url.trim())) next.url = t("urlInvalid");
    setErrors(next);
    if (next.name || next.url) {
      (next.name ? nameRef : urlRef).current?.focus();
      return;
    }
    setSaving(true);
    try {
      await onAdd({ host_asset_id: device, name: name.trim(), url: url.trim(), category });
      onClose();
    } catch (err) {
      setErrors({ form: readableError(err, t("failed")) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <form className="sv-sheet sv-add" aria-label={t("label")} noValidate onSubmit={submit} onKeyDown={(e) => { if (e.key === "Escape") onClose(); }}>
      <div className="sv-add-g">
        <div className="sv-field">
          <label htmlFor="sv-add-name">{t("name")}</label>
          <input ref={nameRef} id="sv-add-name" className="sv-input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="off"
            aria-invalid={errors.name ? true : undefined} aria-describedby={errors.name ? "sv-add-name-err" : undefined} />
          {errors.name && <p className="sv-err" id="sv-add-name-err">{errors.name}</p>}
        </div>
        <div className="sv-field">
          <label htmlFor="sv-add-url">{t("url")}</label>
          <input ref={urlRef} id="sv-add-url" className="sv-input mono" value={url} onChange={(e) => setURL(e.target.value)} placeholder="https://" autoComplete="off"
            aria-invalid={errors.url ? true : undefined} aria-describedby={errors.url ? "sv-add-url-err" : undefined} />
          {errors.url && <p className="sv-err" id="sv-add-url-err">{errors.url}</p>}
        </div>
        <div className="sv-field">
          <label htmlFor="sv-add-device">{t("device")}</label>
          <select id="sv-add-device" className="sv-input" value={device} onChange={(e) => setDevice(e.target.value)}>
            <option value="">{t("noDevice")}</option>
            {devices.map(([id, label]) => <option key={id} value={id}>{label}</option>)}
          </select>
        </div>
        <div className="sv-field">
          <label htmlFor="sv-add-cat">{t("category")}</label>
          <select id="sv-add-cat" className="sv-input" value={category} onChange={(e) => setCategory(e.target.value)}>
            {CATEGORY_ORDER.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </div>
      </div>
      {errors.form && <p className="sv-err sv-form-err" role="alert">{errors.form}</p>}
      <div className="sv-foot">
        <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>{t("cancel")}</Button>
        <Button type="submit" variant="primary" loading={saving}>{t("submit")}</Button>
      </div>
    </form>
  );
}
