"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input } from "../../../../components/ui/Input";
import type { NotificationChannel } from "../../../../hooks/useNotificationChannels";

type EditChannelDialogProps = {
  channel: NotificationChannel | null;
  onClose: () => void;
  onConfirm: (id: string, payload: Record<string, unknown>) => Promise<void>;
};

type ChannelType = "slack" | "email" | "webhook" | "ntfy" | "gotify";

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[10px] text-[var(--muted)]">{label}</span>
      {children}
    </label>
  );
}

function asString(v: unknown): string {
  return typeof v === "string" ? v : typeof v === "number" ? String(v) : "";
}

export function EditChannelDialog({ channel, onClose, onConfirm }: EditChannelDialogProps) {
  const t = useTranslations("notifications");

  const [name, setName] = useState("");
  const [config, setConfig] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!channel) return;
    setName(channel.name);
    const stringified: Record<string, string> = {};
    for (const [k, v] of Object.entries(channel.config)) {
      stringified[k] = asString(v);
    }
    setConfig(stringified);
    setError("");
  }, [channel]);

  if (!channel) return null;

  const type = channel.type as ChannelType;

  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setConfig((prev) => ({ ...prev, [key]: e.target.value }));

  const handleClose = () => {
    if (saving) return;
    setError("");
    onClose();
  };

  const handleSave = async () => {
    if (!name.trim()) {
      setError(t("errors.nameRequired"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      const cleanConfig: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(config)) {
        if (v.trim() !== "") cleanConfig[k] = v.trim();
      }
      await onConfirm(channel.id, { name: name.trim(), config: cleanConfig });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update channel.");
      setSaving(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={handleClose}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[32rem] max-w-[92vw] space-y-4 max-h-[90vh] overflow-y-auto">
          <h3 className="text-sm font-medium text-[var(--text)]">{t(`typeSelect.${type}.name` as Parameters<typeof t>[0])}</h3>

          <div className="space-y-3">
            <FormField label={t("form.name")}>
              <Input value={name} onChange={(e) => setName(e.target.value)} disabled={saving} autoFocus />
            </FormField>

            {type === "slack" && (
              <FormField label={t("form.webhookUrl")}>
                <Input value={config.webhook_url ?? ""} onChange={set("webhook_url")} placeholder={t("form.keepSecret")} disabled={saving} />
              </FormField>
            )}

            {type === "email" && (
              <>
                <FormField label={t("form.smtpHost")}>
                  <Input value={config.smtp_host ?? ""} onChange={set("smtp_host")} disabled={saving} />
                </FormField>
                <FormField label={t("form.smtpPort")}>
                  <Input value={config.smtp_port ?? ""} onChange={set("smtp_port")} disabled={saving} />
                </FormField>
                <FormField label={t("form.smtpUser")}>
                  <Input value={config.smtp_user ?? ""} onChange={set("smtp_user")} disabled={saving} />
                </FormField>
                <FormField label={t("form.smtpPass")}>
                  <Input type="password" value={config.smtp_pass ?? ""} onChange={set("smtp_pass")} placeholder={t("form.keepSecret")} disabled={saving} />
                </FormField>
                <FormField label={t("form.from")}>
                  <Input value={config.from ?? ""} onChange={set("from")} disabled={saving} />
                </FormField>
              </>
            )}

            {type === "webhook" && (
              <FormField label={t("form.url")}>
                <Input value={config.url ?? ""} onChange={set("url")} disabled={saving} />
              </FormField>
            )}

            {type === "ntfy" && (
              <>
                <FormField label={t("form.serverUrl")}>
                  <Input value={config.server_url ?? ""} onChange={set("server_url")} disabled={saving} />
                </FormField>
                <FormField label={t("form.topic")}>
                  <Input value={config.topic ?? ""} onChange={set("topic")} disabled={saving} />
                </FormField>
                <FormField label={t("form.username")}>
                  <Input value={config.username ?? ""} onChange={set("username")} disabled={saving} />
                </FormField>
                <FormField label={t("form.password")}>
                  <Input type="password" value={config.password ?? ""} onChange={set("password")} placeholder={t("form.keepSecret")} disabled={saving} />
                </FormField>
                <FormField label={t("form.token")}>
                  <Input value={config.token ?? ""} onChange={set("token")} placeholder={t("form.keepSecret")} disabled={saving} />
                </FormField>
                <FormField label={t("form.priority")}>
                  <Input value={config.priority ?? ""} onChange={set("priority")} disabled={saving} />
                </FormField>
              </>
            )}

            {type === "gotify" && (
              <>
                <FormField label={t("form.serverUrl")}>
                  <Input value={config.server_url ?? ""} onChange={set("server_url")} disabled={saving} />
                </FormField>
                <FormField label={t("form.appToken")}>
                  <Input value={config.app_token ?? ""} onChange={set("app_token")} placeholder={t("form.keepSecret")} disabled={saving} />
                </FormField>
                <FormField label={t("form.priority")}>
                  <Input value={config.priority ?? ""} onChange={set("priority")} disabled={saving} />
                </FormField>
              </>
            )}
          </div>

          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={handleClose} disabled={saving}>{t("cancel")}</Button>
            <Button variant="primary" loading={saving} onClick={() => { void handleSave(); }}>{t("save")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
