"use client";

import { useState } from "react";
import { Bell, Globe, Mail, MessageSquare } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input } from "../../../../components/ui/Input";

type ChannelType = "slack" | "email" | "webhook" | "ntfy" | "gotify";

type AddChannelDialogProps = {
  open: boolean;
  onClose: () => void;
  onConfirm: (payload: Record<string, unknown>) => Promise<void>;
};

type TypeOption = {
  type: ChannelType;
  icon: React.ReactNode;
  descKey: string;
};

const TYPE_OPTIONS: TypeOption[] = [
  { type: "slack", icon: <MessageSquare size={18} />, descKey: "slack" },
  { type: "email", icon: <Mail size={18} />, descKey: "email" },
  { type: "webhook", icon: <Globe size={18} />, descKey: "webhook" },
  { type: "ntfy", icon: <Bell size={18} />, descKey: "ntfy" },
  { type: "gotify", icon: <Bell size={18} />, descKey: "gotify" },
];

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[10px] text-[var(--muted)]">{label}</span>
      {children}
    </label>
  );
}

type ConfigFormProps = {
  t: ReturnType<typeof useTranslations<"notifications">>;
  type: ChannelType;
  name: string;
  setName: (v: string) => void;
  config: Record<string, string>;
  setConfig: (updater: (prev: Record<string, string>) => Record<string, string>) => void;
  saving: boolean;
};

function ConfigForm({ t, type, name, setName, config, setConfig, saving }: ConfigFormProps) {
  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setConfig((prev) => ({ ...prev, [key]: e.target.value }));

  return (
    <div className="space-y-3">
      <FormField label={t("form.name")}>
        <Input value={name} onChange={(e) => setName(e.target.value)} disabled={saving} autoFocus />
      </FormField>

      {type === "slack" && (
        <FormField label={t("form.webhookUrl")}>
          <Input value={config.webhook_url ?? ""} onChange={set("webhook_url")} placeholder="https://hooks.slack.com/services/…" disabled={saving} />
        </FormField>
      )}

      {type === "email" && (
        <>
          <FormField label={t("form.smtpHost")}>
            <Input value={config.smtp_host ?? ""} onChange={set("smtp_host")} placeholder="smtp.example.com" disabled={saving} />
          </FormField>
          <FormField label={t("form.smtpPort")}>
            <Input value={config.smtp_port ?? "587"} onChange={set("smtp_port")} placeholder="587" disabled={saving} />
          </FormField>
          <FormField label={t("form.smtpUser")}>
            <Input value={config.smtp_user ?? ""} onChange={set("smtp_user")} placeholder="alerts@example.com" disabled={saving} />
          </FormField>
          <FormField label={t("form.smtpPass")}>
            <Input type="password" value={config.smtp_pass ?? ""} onChange={set("smtp_pass")} disabled={saving} />
          </FormField>
          <FormField label={t("form.from")}>
            <Input value={config.from ?? ""} onChange={set("from")} placeholder="LabTether <alerts@example.com>" disabled={saving} />
          </FormField>
        </>
      )}

      {type === "webhook" && (
        <FormField label={t("form.url")}>
          <Input value={config.url ?? ""} onChange={set("url")} placeholder="https://example.com/hooks/labtether" disabled={saving} />
        </FormField>
      )}

      {type === "ntfy" && (
        <>
          <FormField label={t("form.serverUrl")}>
            <Input value={config.server_url ?? "https://ntfy.sh"} onChange={set("server_url")} placeholder="https://ntfy.sh" disabled={saving} />
          </FormField>
          <FormField label={t("form.topic")}>
            <Input value={config.topic ?? ""} onChange={set("topic")} placeholder="labtether-alerts" disabled={saving} />
          </FormField>
          <FormField label={t("form.username")}>
            <Input value={config.username ?? ""} onChange={set("username")} disabled={saving} />
          </FormField>
          <FormField label={t("form.password")}>
            <Input type="password" value={config.password ?? ""} onChange={set("password")} disabled={saving} />
          </FormField>
          <FormField label={t("form.token")}>
            <Input value={config.token ?? ""} onChange={set("token")} disabled={saving} />
          </FormField>
          <FormField label={t("form.priority")}>
            <Input value={config.priority ?? ""} onChange={set("priority")} placeholder="3" disabled={saving} />
          </FormField>
        </>
      )}

      {type === "gotify" && (
        <>
          <FormField label={t("form.serverUrl")}>
            <Input value={config.server_url ?? ""} onChange={set("server_url")} placeholder="https://gotify.example.com" disabled={saving} />
          </FormField>
          <FormField label={t("form.appToken")}>
            <Input value={config.app_token ?? ""} onChange={set("app_token")} disabled={saving} />
          </FormField>
          <FormField label={t("form.priority")}>
            <Input value={config.priority ?? ""} onChange={set("priority")} placeholder="5" disabled={saving} />
          </FormField>
        </>
      )}
    </div>
  );
}

function validateConfig(type: ChannelType, name: string, config: Record<string, string>, t: ReturnType<typeof useTranslations<"notifications">>): string {
  if (!name.trim()) return t("errors.nameRequired");
  if (type === "slack" && !config.webhook_url?.trim()) return t("errors.webhookUrlRequired");
  if (type === "email") {
    if (!config.smtp_host?.trim()) return t("errors.smtpHostRequired");
    if (!config.smtp_port?.trim()) return t("errors.smtpPortRequired");
    if (!config.smtp_user?.trim()) return t("errors.smtpUserRequired");
    if (!config.from?.trim()) return t("errors.fromRequired");
  }
  if (type === "webhook" && !config.url?.trim()) return t("errors.urlRequired");
  if (type === "ntfy") {
    if (!config.server_url?.trim()) return t("errors.serverUrlRequired");
    if (!config.topic?.trim()) return t("errors.topicRequired");
  }
  if (type === "gotify") {
    if (!config.server_url?.trim()) return t("errors.serverUrlRequired");
    if (!config.app_token?.trim()) return t("errors.appTokenRequired");
  }
  return "";
}

export function AddChannelDialog({ open, onClose, onConfirm }: AddChannelDialogProps) {
  const t = useTranslations("notifications");
  const [selectedType, setSelectedType] = useState<ChannelType | null>(null);
  const [name, setName] = useState("");
  const [config, setConfig] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  if (!open) return null;

  const handleClose = () => {
    if (saving) return;
    setSelectedType(null);
    setName("");
    setConfig({});
    setError("");
    onClose();
  };

  const handleBack = () => {
    if (saving) return;
    setSelectedType(null);
    setName("");
    setConfig({});
    setError("");
  };

  const handleSave = async () => {
    if (!selectedType) return;
    const validationError = validateConfig(selectedType, name, config, t);
    if (validationError) {
      setError(validationError);
      return;
    }
    setSaving(true);
    setError("");
    try {
      const cleanConfig: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(config)) {
        if (v.trim() !== "") cleanConfig[k] = v.trim();
      }
      await onConfirm({ name: name.trim(), type: selectedType, config: cleanConfig, enabled: true });
      setSelectedType(null);
      setName("");
      setConfig({});
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create channel.");
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
          {selectedType === null ? (
            <>
              <h3 className="text-sm font-medium text-[var(--text)]">{t("typeSelect.heading")}</h3>
              <div className="grid grid-cols-1 gap-2">
                {TYPE_OPTIONS.map((opt) => (
                  <button
                    key={opt.type}
                    className="flex items-center gap-3 rounded-lg border border-[var(--line)] px-3 py-2.5 text-left transition-colors hover:border-[var(--accent)] hover:bg-[var(--hover)] cursor-pointer bg-transparent"
                    onClick={() => setSelectedType(opt.type)}
                  >
                    <span className="shrink-0 text-[var(--muted)]">{opt.icon}</span>
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-[var(--text)]">{t(`typeSelect.${opt.descKey}.name` as Parameters<typeof t>[0])}</p>
                      <p className="text-xs text-[var(--muted)] truncate">{t(`typeSelect.${opt.descKey}.description` as Parameters<typeof t>[0])}</p>
                    </div>
                  </button>
                ))}
              </div>
              <div className="flex justify-end">
                <Button variant="secondary" onClick={handleClose}>{t("cancel")}</Button>
              </div>
            </>
          ) : (
            <>
              <h3 className="text-sm font-medium text-[var(--text)]">{t(`typeSelect.${selectedType}.name` as Parameters<typeof t>[0])}</h3>
              <ConfigForm
                t={t}
                type={selectedType}
                name={name}
                setName={setName}
                config={config}
                setConfig={setConfig}
                saving={saving}
              />
              {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
              <div className="flex items-center justify-between gap-2">
                <Button variant="ghost" onClick={handleBack} disabled={saving}>{t("back")}</Button>
                <div className="flex items-center gap-2">
                  <Button variant="secondary" onClick={handleClose} disabled={saving}>{t("cancel")}</Button>
                  <Button variant="primary" loading={saving} onClick={() => { void handleSave(); }}>{t("save")}</Button>
                </div>
              </div>
            </>
          )}
        </Card>
      </div>
    </div>
  );
}
