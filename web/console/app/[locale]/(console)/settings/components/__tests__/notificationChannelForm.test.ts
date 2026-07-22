import { describe, expect, it } from "vitest";

import {
  buildNotificationChannelConfig,
  channelConfigForForm,
  configWithSMTPMode,
  normalizeEmailRecipients,
  SMTP_INSECURE_ACKNOWLEDGEMENT,
  validateNotificationChannelForm,
} from "../notificationChannelForm";

describe("notification channel email form", () => {
  it("normalizes comma- and semicolon-delimited recipients", () => {
    expect(normalizeEmailRecipients(" ops@example.com ; oncall@example.net, ")).toBe(
      "ops@example.com, oncall@example.net",
    );
  });

  it("requires recipients and a valid SMTP port", () => {
    const config = {
      smtp_host: "smtp.example.test",
      smtp_port: "587",
      smtp_tls_mode: "starttls",
      from: "alerts@example.test",
    };

    expect(validateNotificationChannelForm("email", "Operations", config)).toBe("recipientsRequired");
    expect(validateNotificationChannelForm("email", "Operations", { ...config, to: "not-an-address" })).toBe(
      "recipientsInvalid",
    );
    expect(
      validateNotificationChannelForm("email", "Operations", {
        ...config,
        smtp_port: "65536",
        to: "ops@example.test",
      }),
    ).toBe("smtpPortInvalid");
  });

  it("builds a canonical, typed STARTTLS payload", () => {
    const config = {
      smtp_host: "smtp.example.test",
      smtp_port: "587",
      smtp_tls_mode: "starttls",
      smtp_user: "alerts@example.test",
      smtp_pass: "replacement-password",
      from: "alerts@example.test",
      to: " ops@example.test ; oncall@example.test ",
    };

    expect(validateNotificationChannelForm("email", "Operations", config)).toBeNull();
    expect(buildNotificationChannelConfig("email", config)).toEqual({
      smtp_host: "smtp.example.test",
      smtp_port: 587,
      smtp_tls_mode: "starttls",
      smtp_user: "alerts@example.test",
      smtp_pass: "replacement-password",
      from: "alerts@example.test",
      to: "ops@example.test, oncall@example.test",
      allow_insecure_smtp: false,
    });
  });

  it("preserves an omitted stored password and requires one when the username changes", () => {
    const originalConfig = {
      smtp_host: "smtp.example.test",
      smtp_port: 587,
      smtp_tls_mode: "starttls",
      smtp_user: "alerts@example.test",
      from: "alerts@example.test",
      to: "ops@example.test",
    };
    const unchanged = {
      smtp_host: "smtp.example.test",
      smtp_port: "587",
      smtp_tls_mode: "starttls",
      smtp_user: "alerts@example.test",
      smtp_pass: "",
      from: "alerts@example.test",
      to: "ops@example.test",
    };

    expect(
      validateNotificationChannelForm("email", "Operations", unchanged, {
        editing: true,
        originalConfig,
      }),
    ).toBeNull();
    expect(
      buildNotificationChannelConfig("email", unchanged, {
        editing: true,
        originalConfig,
      }),
    ).not.toHaveProperty("smtp_pass");

    expect(
      validateNotificationChannelForm("email", "Operations", { ...unchanged, smtp_user: "new@example.test" }, {
        editing: true,
        originalConfig,
      }),
    ).toBe("smtpPassRequired");

    expect(
      buildNotificationChannelConfig("email", { ...unchanged, smtp_user: "" }, {
        editing: true,
        originalConfig,
      }),
    ).toHaveProperty("smtp_pass", null);
  });

  it("never hydrates or re-submits secrets returned by an older backend", () => {
    const legacyEmailConfig = {
      smtp_host: "smtp.example.test",
      smtp_port: 587,
      smtp_user: "alerts@example.test",
      smtp_pass: "legacy-plaintext-password",
      from: "alerts@example.test",
      to: "ops@example.test",
    };
    const emailForm = channelConfigForForm("email", legacyEmailConfig);
    expect(emailForm).not.toHaveProperty("smtp_pass");
    expect(
      buildNotificationChannelConfig("email", emailForm, {
        editing: true,
        originalConfig: legacyEmailConfig,
      }),
    ).not.toHaveProperty("smtp_pass");

    const legacySlackConfig = {
      webhook_url: "https://hooks.example.test/services/legacy/plaintext",
      api_token: "legacy-api-token",
      webhookUrl: "https://hooks.example.test/services/camel/plaintext",
      apiToken: "legacy-camel-api-token",
      clientSecret: "legacy-camel-client-secret",
      APIKey: "legacy-camel-api-key",
      channel: "operations",
    };
    expect(channelConfigForForm("slack", legacySlackConfig)).toEqual({ channel: "operations" });
    expect(
      buildNotificationChannelConfig("slack", { channel: "operations" }, {
        editing: true,
        originalConfig: legacySlackConfig,
      }),
    ).toEqual({ channel: "operations" });
  });

  it("allows redacted required secrets to remain omitted during an edit", () => {
    expect(validateNotificationChannelForm("slack", "Operations Slack", {}, { editing: true })).toBeNull();
    expect(
      validateNotificationChannelForm(
        "webhook",
        "Automation webhook",
        {},
        { editing: true },
      ),
    ).toBeNull();
    expect(
      validateNotificationChannelForm(
        "gotify",
        "Gotify",
        { server_url: "https://gotify.example.test" },
        { editing: true },
      ),
    ).toBeNull();
  });

  it("requires backend permission and an exact acknowledgement for insecure SMTP", () => {
    const config = {
      smtp_host: "smtp.lan",
      smtp_port: "25",
      smtp_tls_mode: "insecure",
      from: "alerts@labtether.local",
      to: "ops@labtether.local",
    };

    expect(validateNotificationChannelForm("email", "LAN SMTP", config)).toBe("smtpInsecurePolicyBlocked");
    expect(
      validateNotificationChannelForm("email", "LAN SMTP", config, { allowInsecureSMTP: true }),
    ).toBe("smtpInsecureAckRequired");
    expect(
      validateNotificationChannelForm(
        "email",
        "LAN SMTP",
        { ...config, smtp_user: "operator", smtp_pass: "unsafe-secret" },
        {
          allowInsecureSMTP: true,
          insecureAcknowledgement: SMTP_INSECURE_ACKNOWLEDGEMENT,
        },
      ),
    ).toBe("smtpInsecureCredentialsForbidden");

    const options = {
      allowInsecureSMTP: true,
      insecureAcknowledgement: SMTP_INSECURE_ACKNOWLEDGEMENT,
    };
    expect(validateNotificationChannelForm("email", "LAN SMTP", config, options)).toBeNull();
    const payload = buildNotificationChannelConfig("email", config, options);
    expect(payload).toMatchObject({
      smtp_port: 25,
      smtp_tls_mode: "insecure",
      allow_insecure_smtp: true,
    });
    expect(JSON.stringify(payload)).not.toContain(SMTP_INSECURE_ACKNOWLEDGEMENT);
    expect(payload).not.toHaveProperty("smtp_user");
    expect(payload).not.toHaveProperty("smtp_pass");
  });

  it("moves between conventional STARTTLS and implicit TLS ports", () => {
    expect(configWithSMTPMode({ smtp_port: "587", smtp_tls_mode: "starttls" }, "implicit")).toEqual({
      smtp_port: "465",
      smtp_tls_mode: "implicit",
    });
    expect(configWithSMTPMode({ smtp_port: "465", smtp_tls_mode: "implicit" }, "starttls")).toEqual({
      smtp_port: "587",
      smtp_tls_mode: "starttls",
    });
    expect(configWithSMTPMode({ smtp_port: "2525", smtp_tls_mode: "starttls" }, "implicit")).toEqual({
      smtp_port: "2525",
      smtp_tls_mode: "implicit",
    });
  });
});
