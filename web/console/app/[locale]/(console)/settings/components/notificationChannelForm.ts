export type NotificationChannelType = "slack" | "email" | "webhook" | "ntfy" | "gotify";

export type SMTPTLSMode = "starttls" | "implicit" | "insecure";

export const SMTP_INSECURE_ACKNOWLEDGEMENT = "INSECURE SMTP";

export type NotificationChannelFormError =
  | "nameRequired"
  | "webhookUrlRequired"
  | "smtpHostRequired"
  | "smtpPortRequired"
  | "smtpPortInvalid"
  | "smtpUserRequired"
  | "smtpPassRequired"
  | "fromRequired"
  | "recipientsRequired"
  | "recipientsInvalid"
  | "smtpInsecurePolicyBlocked"
  | "smtpInsecureCredentialsForbidden"
  | "smtpInsecureAckRequired"
  | "urlRequired"
  | "serverUrlRequired"
  | "topicRequired"
  | "appTokenRequired";

type ChannelFormOptions = {
  editing?: boolean;
  originalConfig?: Record<string, unknown>;
  allowInsecureSMTP?: boolean;
  insecureAcknowledgement?: string;
};

const simpleMailboxPattern = /^[^\s@<>,;]+@[^\s@<>,;]+$/u;

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : typeof value === "number" ? String(value) : "";
}

function normalizedConfigKey(key: string): string {
  return key
    .trim()
    .replace(/([A-Z]+)([A-Z][a-z])/gu, "$1_$2")
    .replace(/([a-z0-9])([A-Z])/gu, "$1_$2")
    .toLowerCase()
    .replace(/[^a-z0-9]+/gu, "_")
    .replace(/^_+|_+$/gu, "");
}

function isSecretConfigKey(type: NotificationChannelType, rawKey: string): boolean {
  const key = normalizedConfigKey(rawKey);
  if (type === "slack" && key === "webhook_url") return true;
  if (type === "webhook" && ["url", "secret", "headers"].includes(key)) return true;
  if (type === "email" && key === "smtp_pass") return true;
  if (type === "ntfy" && ["token", "password"].includes(key)) return true;
  if (type === "gotify" && ["app_token", "token"].includes(key)) return true;
  if (["pass", "passwd", "password", "secret", "token", "api_key", "apikey", "authorization", "auth_header", "private_key", "client_secret", "access_token", "refresh_token", "bearer_token"].includes(key)) {
    return true;
  }
  return ["_password", "_passwd", "_secret", "_token", "_api_key", "_private_key"].some((suffix) =>
    key.endsWith(suffix),
  );
}

function safeOriginalConfig(type: NotificationChannelType, config: Record<string, unknown>): Record<string, unknown> {
  const safe: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(config)) {
    if (isSecretConfigKey(type, key)) continue;
    if (typeof value === "string" && value.trim().startsWith("v2:")) continue;
    safe[key] = value;
  }
  return safe;
}

export function normalizeEmailRecipients(value: string): string {
  return value
    .split(/[;,]/u)
    .map((entry) => entry.trim())
    .filter(Boolean)
    .join(", ");
}

function emailRecipientsAreValid(value: string): boolean {
  const normalized = normalizeEmailRecipients(value);
  if (!normalized) return false;
  return normalized.split(", ").every((address) => simpleMailboxPattern.test(address));
}

export function smtpTLSModeFromConfig(config: Record<string, unknown> | Record<string, string>): SMTPTLSMode {
  const explicit = stringValue(config.smtp_tls_mode).toLowerCase();
  if (explicit === "implicit" || explicit === "implicit_tls" || explicit === "tls" || explicit === "smtps") {
    return "implicit";
  }
  if (explicit === "insecure" || explicit === "none" || explicit === "plaintext") {
    return "insecure";
  }
  if (explicit === "starttls") return "starttls";
  return stringValue(config.smtp_port) === "465" ? "implicit" : "starttls";
}

export function defaultChannelConfig(type: NotificationChannelType): Record<string, string> {
  if (type === "email") {
    return { smtp_port: "587", smtp_tls_mode: "starttls" };
  }
  if (type === "ntfy") {
    return { server_url: "https://ntfy.sh" };
  }
  return {};
}

export function channelConfigForForm(type: NotificationChannelType, config: Record<string, unknown>): Record<string, string> {
  const formConfig: Record<string, string> = {};
  for (const [key, value] of Object.entries(config)) {
    // Never hydrate a secret into a browser input, even if an older backend
    // accidentally returned plaintext or an encrypted storage value.
    if (isSecretConfigKey(type, key)) continue;
    if (typeof value === "string" && value.trim().startsWith("v2:")) continue;
    const normalized = stringValue(value);
    if (normalized) formConfig[key] = normalized;
  }
  if (type === "email") {
    formConfig.to = stringValue(config.to) || stringValue(config.recipients) || stringValue(config.email_to);
    formConfig.smtp_tls_mode = smtpTLSModeFromConfig(config);
    if (!formConfig.smtp_port) {
      formConfig.smtp_port = formConfig.smtp_tls_mode === "implicit" ? "465" : "587";
    }
    delete formConfig.allow_insecure_smtp;
    delete formConfig.recipients;
    delete formConfig.email_to;
  }
  return formConfig;
}

export function configWithSMTPMode(config: Record<string, string>, mode: SMTPTLSMode): Record<string, string> {
  const previousMode = smtpTLSModeFromConfig(config);
  const currentPort = config.smtp_port?.trim() ?? "";
  let nextPort = currentPort;
  if (mode === "implicit" && (currentPort === "" || (previousMode === "starttls" && currentPort === "587"))) {
    nextPort = "465";
  } else if (mode === "starttls" && (currentPort === "" || (previousMode === "implicit" && currentPort === "465"))) {
    nextPort = "587";
  }
  return { ...config, smtp_tls_mode: mode, smtp_port: nextPort };
}

export function validateNotificationChannelForm(
  type: NotificationChannelType,
  name: string,
  config: Record<string, string>,
  options: ChannelFormOptions = {},
): NotificationChannelFormError | null {
  if (!name.trim()) return "nameRequired";
  if (type === "slack" && !config.webhook_url?.trim() && !options.editing) return "webhookUrlRequired";
  if (type === "email") {
    if (!config.smtp_host?.trim()) return "smtpHostRequired";
    const portText = config.smtp_port?.trim() ?? "";
    if (!portText) return "smtpPortRequired";
    const port = Number(portText);
    if (!Number.isInteger(port) || port < 1 || port > 65535) return "smtpPortInvalid";
    if (!config.from?.trim()) return "fromRequired";
    if (!normalizeEmailRecipients(config.to ?? "")) return "recipientsRequired";
    if (!emailRecipientsAreValid(config.to ?? "")) return "recipientsInvalid";

    const user = config.smtp_user?.trim() ?? "";
    const password = config.smtp_pass?.trim() ?? "";
    const originalUser = stringValue(options.originalConfig?.smtp_user);
    if (password && !user) return "smtpUserRequired";
    if (user && !password && (!options.editing || user !== originalUser)) return "smtpPassRequired";

    if (smtpTLSModeFromConfig(config) === "insecure") {
      if (!options.allowInsecureSMTP) return "smtpInsecurePolicyBlocked";
      if (user || password) return "smtpInsecureCredentialsForbidden";
      if (options.insecureAcknowledgement?.trim() !== SMTP_INSECURE_ACKNOWLEDGEMENT) {
        return "smtpInsecureAckRequired";
      }
    }
  }
  if (type === "webhook" && !config.url?.trim() && !options.editing) return "urlRequired";
  if (type === "ntfy") {
    if (!config.server_url?.trim()) return "serverUrlRequired";
    if (!config.topic?.trim()) return "topicRequired";
  }
  if (type === "gotify") {
    if (!config.server_url?.trim()) return "serverUrlRequired";
    if (!config.app_token?.trim() && !options.editing) return "appTokenRequired";
  }
  return null;
}

export function buildNotificationChannelConfig(
  type: NotificationChannelType,
  config: Record<string, string>,
  options: ChannelFormOptions = {},
): Record<string, unknown> {
  const cleanConfig = safeOriginalConfig(type, options.originalConfig ?? {});
  for (const [key, value] of Object.entries(config)) {
    if (type === "email" && (key === "allow_insecure_smtp" || key === "recipients" || key === "email_to")) {
      continue;
    }
    const normalized = value.trim();
    if (normalized) cleanConfig[key] = normalized;
    else delete cleanConfig[key];
  }

  if (type === "email") {
    delete cleanConfig.recipients;
    delete cleanConfig.email_to;
    cleanConfig.to = normalizeEmailRecipients(config.to ?? "");
    cleanConfig.smtp_port = Number(config.smtp_port?.trim());
    const tlsMode = smtpTLSModeFromConfig(config);
    cleanConfig.smtp_tls_mode = tlsMode;
    cleanConfig.allow_insecure_smtp =
      tlsMode === "insecure" &&
      options.allowInsecureSMTP === true &&
      options.insecureAcknowledgement?.trim() === SMTP_INSECURE_ACKNOWLEDGEMENT;

    const originalUser = stringValue(options.originalConfig?.smtp_user);
    const nextUser = config.smtp_user?.trim() ?? "";
    const nextPassword = config.smtp_pass?.trim() ?? "";
    if (options.editing && originalUser && !nextUser && !nextPassword) {
      // The API intentionally omits stored passwords. Explicitly clear it when
      // the operator removes the associated username; otherwise omission means
      // preserve the encrypted secret.
      cleanConfig.smtp_pass = null;
    }
  }

  return cleanConfig;
}
