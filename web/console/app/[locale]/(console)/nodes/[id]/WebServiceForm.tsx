"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import type { WebService } from "../../../../hooks/useWebServices";

const CATEGORIES = [
  "dashboard", "monitoring", "storage", "media", "network",
  "automation", "development", "security", "database", "other",
] as const;

type WebServiceFormProps = {
  initial?: WebService;
  onSave: (data: { name: string; url: string; category: string }) => void | Promise<void>;
  onCancel: () => void;
};

/** Add or edit a web address on this device: name, URL and category. */
export function WebServiceForm({ initial, onSave, onCancel }: WebServiceFormProps) {
  const t = useTranslations("devices.settingsTab.web");
  const [name, setName] = useState(initial?.name ?? "");
  const [url, setUrl] = useState(initial?.url ?? "");
  const [category, setCategory] = useState(initial?.category ?? "other");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<{ field: "name" | "url" | "form"; text: string } | null>(null);

  const handleSubmit = async () => {
    setError(null);
    if (!name.trim()) { setError({ field: "name", text: t("nameRequired") }); return; }
    if (!url.trim()) { setError({ field: "url", text: t("urlRequired") }); return; }
    try {
      new URL(url);
    } catch {
      setError({ field: "url", text: t("urlInvalid") });
      return;
    }
    setSaving(true);
    try {
      await onSave({ name: name.trim(), url: url.trim(), category });
    } catch (e) {
      setError({ field: "form", text: e instanceof Error ? e.message : t("saveFailed") });
    } finally {
      setSaving(false);
    }
  };

  const fieldError = (field: "name" | "url") =>
    error?.field === field ? <p className="dv-err" id={`dst-web-${field}-err`}>{error.text}</p> : null;

  return (
    <div
      className="dv-form"
      onKeyDown={(event) => {
        if (event.key === "Enter" && (event.target as HTMLElement).tagName === "INPUT") {
          event.preventDefault();
          void handleSubmit();
        }
      }}
    >
      <div className="dv-form-row">
        <div className="dv-field">
          <label htmlFor="dst-web-name">{t("name")}</label>
          <input
            id="dst-web-name"
            className="dv-input"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder={t("namePlaceholder")}
            aria-invalid={error?.field === "name" ? true : undefined}
            aria-describedby={error?.field === "name" ? "dst-web-name-err" : undefined}
          />
          {fieldError("name")}
        </div>
        <div className="dv-field">
          <label htmlFor="dst-web-url">{t("url")}</label>
          <input
            id="dst-web-url"
            className="dv-input"
            value={url}
            onChange={e => setUrl(e.target.value)}
            placeholder="https://10.20.0.31:9443"
            spellCheck={false}
            aria-invalid={error?.field === "url" ? true : undefined}
            aria-describedby={error?.field === "url" ? "dst-web-url-err" : undefined}
          />
          {fieldError("url")}
        </div>
        <div className="dv-field">
          <label htmlFor="dst-web-cat">{t("category")}</label>
          <select id="dst-web-cat" className="dv-input" value={category} onChange={e => setCategory(e.target.value)}>
            {CATEGORIES.map(c => (
              <option key={c} value={c}>{t(`categories.${c}`)}</option>
            ))}
          </select>
        </div>
      </div>
      {error?.field === "form" ? <p className="dv-err" role="alert">{error.text}</p> : null}
      <div className="dv-form-acts">
        <Button size="sm" variant="ghost" onClick={onCancel} disabled={saving}>{t("cancel")}</Button>
        <Button size="sm" variant="primary" onClick={() => void handleSubmit()} disabled={saving}>
          {saving ? t("saving") : initial ? t("save") : t("add")}
        </Button>
      </div>
    </div>
  );
}
