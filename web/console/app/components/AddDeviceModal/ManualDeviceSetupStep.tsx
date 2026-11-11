"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useToast } from "../../contexts/ToastContext";
import type { AddDeviceAddedEvent } from "./types";

type ManualDeviceSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
};

/** Product names stay as they are; the empty value is "Other" in the reader's language. */
const PLATFORM_OPTIONS = [
  { value: "linux", label: "Linux" },
  { value: "windows", label: "Windows" },
  { value: "macos", label: "macOS" },
];

export function ManualDeviceSetupStep({ onBack, onClose, onAdded }: ManualDeviceSetupStepProps) {
  const t = useTranslations("auth.addDevice");
  const { addToast } = useToast();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [platform, setPlatform] = useState("");
  const [tags, setTags] = useState("");

  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");

  const handleSave = async () => {
    const trimmedName = name.trim();
    const trimmedHost = host.trim();

    if (!trimmedName) {
      const err = t("manual.nameRequired");
      setFormError(err);
      addToast("error", err);
      return;
    }
    if (!trimmedHost) {
      const err = t("manual.hostRequired");
      setFormError(err);
      addToast("error", err);
      return;
    }

    setFormError("");
    setSaving(true);

    try {
      const tagList = tags
        .split(",")
        .map((tag) => tag.trim())
        .filter((tag) => tag.length > 0);

      const body: Record<string, unknown> = {
        name: trimmedName,
        host: trimmedHost,
      };
      if (platform) {
        body.platform = platform;
      }
      if (tagList.length > 0) {
        body.tags = tagList;
      }

      const res = await fetch("/api/assets/manual", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        const payload = (await res.json().catch(() => ({}))) as { error?: string };
        throw new Error(payload.error || "");
      }

      const payload = (await res.json()) as { id?: string; name?: string };
      addToast("success", t("manual.added", { name: trimmedName }));
      onAdded?.({ source: "manual", focusQuery: trimmedName });
      onClose();
      void payload;
    } catch (err) {
      const msg = err instanceof Error && err.message ? err.message : t("manual.addFailed");
      setFormError(msg);
      addToast("error", msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-3">
      {formError ? <p className="text-xs text-[var(--bad)]">{formError}</p> : null}

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("manual.name")}</label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="my-server"
        />
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("manual.host")}</label>
        <Input
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder={t("manual.hostPlaceholder")}
        />
        <p className="mt-1 text-xs text-[var(--muted)]">{t("manual.hostHint")}</p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("manual.platform")}</label>
        <select
          value={platform}
          onChange={(e) => setPlatform(e.target.value)}
          className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-2 text-xs text-[var(--text)]"
        >
          <option value="">{t("manual.otherPlatform")}</option>
          {PLATFORM_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">{t("manual.tags")}</label>
        <Input
          value={tags}
          onChange={(e) => setTags(e.target.value)}
          placeholder="homelab, storage"
        />
        <p className="mt-1 text-xs text-[var(--muted)]">{t("manual.tagsHint")}</p>
      </div>

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>{t("form.back")}</Button>
        <Button
          variant="primary"
          disabled={saving}
          onClick={() => void handleSave()}
        >
          {saving ? t("manual.adding") : t("manual.add")}
        </Button>
      </div>
    </div>
  );
}
