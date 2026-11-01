"use client";

import { useState } from "react";
import { Bell, Globe, Mail, MessageSquare } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import {
  buildNotificationChannelConfig,
  configWithSMTPMode,
  defaultChannelConfig,
  SMTP_INSECURE_ACKNOWLEDGEMENT,
  smtpTLSModeFromConfig,
  validateNotificationChannelForm,
  type NotificationChannelType,
  type SMTPTLSMode,
} from "./notificationChannelForm";
import { SettingsDialog } from "./SettingsKit";

type AddChannelDialogProps = {
  open: boolean;
  allowInsecureSMTP: boolean;
  onClose: () => void;
  onConfirm: (payload: Record<string, unknown>) => Promise<void>;
};

type TypeOption = {
  type: NotificationChannelType;
  icon: React.ReactNode;
  descKey: string;
};

const TYPE_OPTIONS: TypeOption[] = [
  { type: "slack", icon: <MessageSquare size={18} aria-hidden="true" />, descKey: "slack" },
  { type: "email", icon: <Mail size={18} aria-hidden="true" />, descKey: "email" },
  { type: "webhook", icon: <Globe size={18} aria-hidden="true" />, descKey: "webhook" },
  { type: "ntfy", icon: <Bell size={18} aria-hidden="true" />, descKey: "ntfy" },
  { type: "gotify", icon: <Bell size={18} aria-hidden="true" />, descKey: "gotify" },
];

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="st-field">
      <span className="st-lbl">{label}</span>
      {children}
    </label>
  );
}

type ConfigFormProps = {
  t: ReturnType<typeof useTranslations<"notifications">>;
  type: NotificationChannelType;
  name: string;
  setName: (v: string) => void;
  config: Record<string, string>;
  setConfig: (updater: (prev: Record<string, string>) => Record<string, string>) => void;
  allowInsecureSMTP: boolean;
  insecureAcknowledgement: string;
  setInsecureAcknowledgement: (value: string) => void;
  saving: boolean;
};

function ConfigForm({
  t,
  type,
  name,
  setName,
  config,
  setConfig,
  allowInsecureSMTP,
  insecureAcknowledgement,
  setInsecureAcknowledgement,
  saving,
}: ConfigFormProps) {
  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setConfig((prev) => ({ ...prev, [key]: e.target.value }));
  const smtpTLSMode = smtpTLSModeFromConfig(config);

  const setSMTPMode = (mode: SMTPTLSMode) => {
    setConfig((prev) => configWithSMTPMode(prev, mode));
    setInsecureAcknowledgement("");
  };

  return (
    <div className="st-form-1">
      <FormField label={t("form.name")}>
        <Input name="name" value={name} onChange={(e) => setName(e.target.value)} disabled={saving} autoFocus />
      </FormField>

      {type === "slack" && (
        <FormField label={t("form.webhookUrl")}>
          <Input name="webhook_url" type="password" value={config.webhook_url ?? ""} onChange={set("webhook_url")} placeholder="https://hooks.slack.com/services/…" disabled={saving} autoComplete="new-password" />
        </FormField>
      )}

      {type === "email" && (
        <>
          <FormField label={t("form.smtpHost")}>
            <Input name="smtp_host" value={config.smtp_host ?? ""} onChange={set("smtp_host")} placeholder="smtp.example.com" disabled={saving} autoComplete="off" />
          </FormField>
          <FormField label={t("form.smtpPort")}>
            <Input name="smtp_port" value={config.smtp_port ?? "587"} onChange={set("smtp_port")} placeholder="587" inputMode="numeric" disabled={saving} />
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
              {allowInsecureSMTP ? <option value="insecure">{t("form.smtpTLSInsecure")}</option> : null}
            </Select>
          </FormField>
          <FormField label={t("form.smtpUser")}>
            <Input name="smtp_user" value={config.smtp_user ?? ""} onChange={set("smtp_user")} placeholder="alerts@example.com" disabled={saving} autoComplete="username" />
          </FormField>
          <FormField label={t("form.smtpPass")}>
            <Input name="smtp_pass" type="password" value={config.smtp_pass ?? ""} onChange={set("smtp_pass")} disabled={saving} autoComplete="new-password" />
          </FormField>
          <FormField label={t("form.from")}>
            <Input name="from" value={config.from ?? ""} onChange={set("from")} placeholder="LabTether <alerts@example.com>" disabled={saving} />
          </FormField>
          <FormField label={t("form.recipients")}>
            <Input name="to" value={config.to ?? ""} onChange={set("to")} placeholder="ops@example.com, on-call@example.com" inputMode="email" disabled={saving} maxLength={2048} />
          </FormField>
          {smtpTLSMode === "insecure" && allowInsecureSMTP ? (
            <div className="st-form-1">
              <p className="st-msg bad">{t("form.smtpInsecureWarning")}</p>
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
          ) : null}
        </>
      )}

      {type === "webhook" && (
        <FormField label={t("form.url")}>
          <Input name="url" type="password" value={config.url ?? ""} onChange={set("url")} placeholder="https://example.com/hooks/labtether" disabled={saving} autoComplete="new-password" />
        </FormField>
      )}

      {type === "ntfy" && (
        <>
          <FormField label={t("form.serverUrl")}>
            <Input name="server_url" value={config.server_url ?? "https://ntfy.sh"} onChange={set("server_url")} placeholder="https://ntfy.sh" disabled={saving} />
          </FormField>
          <FormField label={t("form.topic")}>
            <Input name="topic" value={config.topic ?? ""} onChange={set("topic")} placeholder="labtether-alerts" disabled={saving} />
          </FormField>
          <FormField label={t("form.username")}>
            <Input name="username" value={config.username ?? ""} onChange={set("username")} disabled={saving} autoComplete="username" />
          </FormField>
          <FormField label={t("form.password")}>
            <Input name="password" type="password" value={config.password ?? ""} onChange={set("password")} disabled={saving} autoComplete="new-password" />
          </FormField>
          <FormField label={t("form.token")}>
            <Input name="token" type="password" value={config.token ?? ""} onChange={set("token")} disabled={saving} autoComplete="new-password" />
          </FormField>
          <FormField label={t("form.priority")}>
            <Input name="priority" value={config.priority ?? ""} onChange={set("priority")} placeholder="3" inputMode="numeric" disabled={saving} />
          </FormField>
        </>
      )}

      {type === "gotify" && (
        <>
          <FormField label={t("form.serverUrl")}>
            <Input name="server_url" value={config.server_url ?? ""} onChange={set("server_url")} placeholder="https://gotify.example.com" disabled={saving} />
          </FormField>
          <FormField label={t("form.appToken")}>
            <Input name="app_token" type="password" value={config.app_token ?? ""} onChange={set("app_token")} disabled={saving} autoComplete="new-password" />
          </FormField>
          <FormField label={t("form.priority")}>
            <Input name="priority" value={config.priority ?? ""} onChange={set("priority")} placeholder="5" inputMode="numeric" disabled={saving} />
          </FormField>
        </>
      )}
    </div>
  );
}

export function AddChannelDialog({ open, allowInsecureSMTP, onClose, onConfirm }: AddChannelDialogProps) {
  const t = useTranslations("notifications");
  const [selectedType, setSelectedType] = useState<NotificationChannelType | null>(null);
  const [name, setName] = useState("");
  const [config, setConfig] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [insecureAcknowledgement, setInsecureAcknowledgement] = useState("");
  const [saving, setSaving] = useState(false);

  if (!open) return null;

  const handleClose = () => {
    if (saving) return;
    setSelectedType(null);
    setName("");
    setConfig({});
    setInsecureAcknowledgement("");
    setError("");
    onClose();
  };

  const handleBack = () => {
    if (saving) return;
    setSelectedType(null);
    setName("");
    setConfig({});
    setInsecureAcknowledgement("");
    setError("");
  };

  const handleSelectType = (type: NotificationChannelType) => {
    setSelectedType(type);
    setConfig(defaultChannelConfig(type));
    setInsecureAcknowledgement("");
    setError("");
  };

  const handleSave = async () => {
    if (!selectedType) return;
    const validationError = validateNotificationChannelForm(selectedType, name, config, {
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
      const cleanConfig = buildNotificationChannelConfig(selectedType, config, {
        allowInsecureSMTP,
        insecureAcknowledgement,
      });
      await onConfirm({ name: name.trim(), type: selectedType, config: cleanConfig, enabled: true });
      setSelectedType(null);
      setName("");
      setConfig({});
      setInsecureAcknowledgement("");
      setSaving(false);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create channel.");
      setSaving(false);
    }
  };

  return (
    <SettingsDialog labelledBy="add-notification-channel-title" onClose={handleClose}>
      {selectedType === null ? (
        <>
          <h3 id="add-notification-channel-title">{t("typeSelect.heading")}</h3>
          <div className="st-pick">
            {TYPE_OPTIONS.map((opt) => (
              <button
                key={opt.type}
                type="button"
                onClick={() => handleSelectType(opt.type)}
              >
                {opt.icon}
                <span className="st-row-l two">
                  <span className="st-nm">{t(`typeSelect.${opt.descKey}.name` as Parameters<typeof t>[0])}</span>
                  <span className="st-g">{t(`typeSelect.${opt.descKey}.description` as Parameters<typeof t>[0])}</span>
                </span>
              </button>
            ))}
          </div>
          <div className="st-dlg-acts">
            <Button variant="secondary" onClick={handleClose}>{t("cancel")}</Button>
          </div>
        </>
      ) : (
        <>
          <h3 id="add-notification-channel-title">{t(`typeSelect.${selectedType}.name` as Parameters<typeof t>[0])}</h3>
          <ConfigForm
            t={t}
            type={selectedType}
            name={name}
            setName={setName}
            config={config}
            setConfig={setConfig}
            allowInsecureSMTP={allowInsecureSMTP}
            insecureAcknowledgement={insecureAcknowledgement}
            setInsecureAcknowledgement={setInsecureAcknowledgement}
            saving={saving}
          />
          {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
          <div className="st-dlg-acts">
            <Button variant="ghost" onClick={handleBack} disabled={saving}>{t("back")}</Button>
            <span className="sp" />
            <Button variant="secondary" onClick={handleClose} disabled={saving}>{t("cancel")}</Button>
            <Button variant="primary" loading={saving} onClick={() => { void handleSave(); }}>{t("save")}</Button>
          </div>
        </>
      )}
    </SettingsDialog>
  );
}
