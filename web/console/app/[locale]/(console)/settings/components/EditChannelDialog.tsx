"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input, Select } from "../../../../components/ui/Input";
import type { NotificationChannel } from "../../../../hooks/useNotificationChannels";
import {
  buildNotificationChannelConfig,
  channelConfigForForm,
  configWithSMTPMode,
  SMTP_INSECURE_ACKNOWLEDGEMENT,
  smtpTLSModeFromConfig,
  validateNotificationChannelForm,
  type NotificationChannelType,
  type SMTPTLSMode,
} from "./notificationChannelForm";

type EditChannelDialogProps = {
  channel: NotificationChannel | null;
  allowInsecureSMTP: boolean;
  onClose: () => void;
  onConfirm: (id: string, payload: Record<string, unknown>) => Promise<void>;
};

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[10px] text-[var(--muted)]">{label}</span>
      {children}
    </label>
  );
}

export function EditChannelDialog({ channel, allowInsecureSMTP, onClose, onConfirm }: EditChannelDialogProps) {
  const t = useTranslations("notifications");

  const [name, setName] = useState("");
  const [config, setConfig] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [insecureAcknowledgement, setInsecureAcknowledgement] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!channel) {
      setSaving(false);
      return;
    }
    setName(channel.name);
    setConfig(channelConfigForForm(channel.type as NotificationChannelType, channel.config));
    setInsecureAcknowledgement("");
    setError("");
    setSaving(false);
  }, [channel]);

  if (!channel) return null;

  const type = channel.type as NotificationChannelType;
  const smtpTLSMode = smtpTLSModeFromConfig(config);

  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setConfig((prev) => ({ ...prev, [key]: e.target.value }));

  const setSMTPMode = (mode: SMTPTLSMode) => {
    setConfig((prev) => configWithSMTPMode(prev, mode));
    setInsecureAcknowledgement("");
  };

  const handleClose = () => {
    if (saving) return;
    setError("");
    setInsecureAcknowledgement("");
    onClose();
  };

  const handleSave = async () => {
    const validationError = validateNotificationChannelForm(type, name, config, {
      editing: true,
      originalConfig: channel.config,
      allowInsecureSMTP,
      insecureAcknowledgement,
    });
    if (validationError) {
      setError(t(`errors.${validationError}` as Parameters<typeof t>[0]));
      return;
    }
    setSaving(true);
    setError("");
    try {
      const cleanConfig = buildNotificationChannelConfig(type, config, {
        editing: true,
        originalConfig: channel.config,
        allowInsecureSMTP,
        insecureAcknowledgement,
      });
      await onConfirm(channel.id, { name: name.trim(), config: cleanConfig });
      setSaving(false);
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
      onKeyDown={(event) => { if (event.key === "Escape") handleClose(); }}
    >
      <div role="dialog" aria-modal="true" aria-labelledby="edit-notification-channel-title" onClick={(e) => e.stopPropagation()}>
        <Card className="w-[32rem] max-w-[92vw] space-y-4 max-h-[90vh] overflow-y-auto">
          <h3 id="edit-notification-channel-title" className="text-sm font-medium text-[var(--text)]">{t(`typeSelect.${type}.name` as Parameters<typeof t>[0])}</h3>

          <div className="space-y-3">
            <FormField label={t("form.name")}>
              <Input name="name" value={name} onChange={(e) => setName(e.target.value)} disabled={saving} autoFocus />
            </FormField>

            {type === "slack" && (
              <FormField label={t("form.webhookUrl")}>
                <Input name="webhook_url" type="password" value={config.webhook_url ?? ""} onChange={set("webhook_url")} placeholder={t("form.keepSecret")} disabled={saving} autoComplete="new-password" />
              </FormField>
            )}

            {type === "email" && (
              <>
                <FormField label={t("form.smtpHost")}>
                  <Input name="smtp_host" value={config.smtp_host ?? ""} onChange={set("smtp_host")} disabled={saving} autoComplete="off" />
                </FormField>
                <FormField label={t("form.smtpPort")}>
                  <Input name="smtp_port" value={config.smtp_port ?? ""} onChange={set("smtp_port")} inputMode="numeric" disabled={saving} />
                </FormField>
                <FormField label={t("form.smtpTLSMode")}>
                  <Select
                    name="smtp_tls_mode"
                    className="w-full"
                    value={smtpTLSMode}
                    onChange={(event) => setSMTPMode(event.target.value as SMTPTLSMode)}
                    disabled={saving}
                  >
                    <option value="starttls">{t("form.smtpTLSStartTLS")}</option>
                    <option value="implicit">{t("form.smtpTLSImplicit")}</option>
                    {allowInsecureSMTP || smtpTLSMode === "insecure" ? (
                      <option value="insecure" disabled={!allowInsecureSMTP}>
                        {allowInsecureSMTP ? t("form.smtpTLSInsecure") : t("form.smtpTLSInsecureBlocked")}
                      </option>
                    ) : null}
                  </Select>
                </FormField>
                <FormField label={t("form.smtpUser")}>
                  <Input name="smtp_user" value={config.smtp_user ?? ""} onChange={set("smtp_user")} disabled={saving} autoComplete="username" />
                </FormField>
                <FormField label={t("form.smtpPass")}>
                  <Input name="smtp_pass" type="password" value={config.smtp_pass ?? ""} onChange={set("smtp_pass")} placeholder={t("form.keepSecret")} disabled={saving} autoComplete="new-password" />
                </FormField>
                <FormField label={t("form.from")}>
                  <Input name="from" value={config.from ?? ""} onChange={set("from")} disabled={saving} />
                </FormField>
                <FormField label={t("form.recipients")}>
                  <Input name="to" value={config.to ?? ""} onChange={set("to")} placeholder="ops@example.com, on-call@example.com" inputMode="email" disabled={saving} maxLength={2048} />
                </FormField>
                {smtpTLSMode === "insecure" ? (
                  allowInsecureSMTP ? (
                    <div className="rounded-lg border border-[var(--bad)] bg-[var(--bg-error)] p-3 space-y-2">
                      <p className="text-xs text-[var(--bad)]">{t("form.smtpInsecureWarning")}</p>
                      <FormField label={t("form.smtpInsecureAcknowledgement", { phrase: SMTP_INSECURE_ACKNOWLEDGEMENT })}>
                        <Input
                          name="smtp_insecure_acknowledgement"
                          value={insecureAcknowledgement}
                          onChange={(event) => setInsecureAcknowledgement(event.target.value)}
                          placeholder={SMTP_INSECURE_ACKNOWLEDGEMENT}
                          autoComplete="off"
                          spellCheck={false}
                          disabled={saving}
                        />
                      </FormField>
                    </div>
                  ) : (
                    <p className="text-xs text-[var(--bad)]">{t("form.smtpInsecurePolicyWarning")}</p>
                  )
                ) : null}
              </>
            )}

            {type === "webhook" && (
              <FormField label={t("form.url")}>
                <Input name="url" type="password" value={config.url ?? ""} onChange={set("url")} placeholder={t("form.keepSecret")} disabled={saving} autoComplete="new-password" />
              </FormField>
            )}

            {type === "ntfy" && (
              <>
                <FormField label={t("form.serverUrl")}>
                  <Input name="server_url" value={config.server_url ?? ""} onChange={set("server_url")} disabled={saving} />
                </FormField>
                <FormField label={t("form.topic")}>
                  <Input name="topic" value={config.topic ?? ""} onChange={set("topic")} disabled={saving} />
                </FormField>
                <FormField label={t("form.username")}>
                  <Input name="username" value={config.username ?? ""} onChange={set("username")} disabled={saving} autoComplete="username" />
                </FormField>
                <FormField label={t("form.password")}>
                  <Input name="password" type="password" value={config.password ?? ""} onChange={set("password")} placeholder={t("form.keepSecret")} disabled={saving} autoComplete="new-password" />
                </FormField>
                <FormField label={t("form.token")}>
                  <Input name="token" type="password" value={config.token ?? ""} onChange={set("token")} placeholder={t("form.keepSecret")} disabled={saving} autoComplete="new-password" />
                </FormField>
                <FormField label={t("form.priority")}>
                  <Input name="priority" value={config.priority ?? ""} onChange={set("priority")} inputMode="numeric" disabled={saving} />
                </FormField>
              </>
            )}

            {type === "gotify" && (
              <>
                <FormField label={t("form.serverUrl")}>
                  <Input name="server_url" value={config.server_url ?? ""} onChange={set("server_url")} disabled={saving} />
                </FormField>
                <FormField label={t("form.appToken")}>
                  <Input name="app_token" type="password" value={config.app_token ?? ""} onChange={set("app_token")} placeholder={t("form.keepSecret")} disabled={saving} autoComplete="new-password" />
                </FormField>
                <FormField label={t("form.priority")}>
                  <Input name="priority" value={config.priority ?? ""} onChange={set("priority")} inputMode="numeric" disabled={saving} />
                </FormField>
              </>
            )}
          </div>

          {error ? <p role="alert" className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={handleClose} disabled={saving}>{t("cancel")}</Button>
            <Button variant="primary" loading={saving} onClick={() => { void handleSave(); }}>{t("save")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
