import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AddChannelDialog } from "../AddChannelDialog";
import { EditChannelDialog } from "../EditChannelDialog";
import { SMTP_INSECURE_ACKNOWLEDGEMENT } from "../notificationChannelForm";

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
  vi.restoreAllMocks();
});

function input(name: string): HTMLInputElement {
  const element = container.querySelector<HTMLInputElement>(`input[name="${name}"]`);
  if (!element) throw new Error(`missing input ${name}`);
  return element;
}

function select(name: string): HTMLSelectElement {
  const element = container.querySelector<HTMLSelectElement>(`select[name="${name}"]`);
  if (!element) throw new Error(`missing select ${name}`);
  return element;
}

function button(text: string): HTMLButtonElement {
  const element = [...container.querySelectorAll<HTMLButtonElement>("button")].find(
    (candidate) => candidate.textContent?.trim() === text,
  );
  if (!element) throw new Error(`missing button ${text}`);
  return element;
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

async function setValue(element: HTMLInputElement | HTMLSelectElement, value: string): Promise<void> {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(element), "value")?.set;
    if (!setter) throw new Error("element value setter is unavailable");
    setter.call(element, value);
    element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  });
}

async function chooseEmailChannel(): Promise<void> {
  const emailButton = [...container.querySelectorAll<HTMLButtonElement>("button")].find((candidate) =>
    candidate.textContent?.includes("typeSelect.email.name"),
  );
  if (!emailButton) throw new Error("missing email channel button");
  await click(emailButton);
}

async function chooseChannel(type: "ntfy" | "gotify"): Promise<void> {
  const channelButton = [...container.querySelectorAll<HTMLButtonElement>("button")].find((candidate) =>
    candidate.textContent?.includes(`typeSelect.${type}.name`),
  );
  if (!channelButton) throw new Error(`missing ${type} channel button`);
  await click(channelButton);
}

async function fillRequiredEmailFields(): Promise<void> {
  await setValue(input("name"), "Operations email");
  await setValue(input("smtp_host"), "smtp.example.test");
  await setValue(input("from"), "alerts@example.test");
  await setValue(input("to"), " ops@example.test ; oncall@example.test ");
}

describe("notification channel dialogs", () => {
  it("exposes an accessible modal and masks ntfy bearer credentials", async () => {
    await act(async () => {
      root.render(
        <AddChannelDialog
          open
          allowInsecureSMTP={false}
          onClose={() => undefined}
          onConfirm={async () => undefined}
        />,
      );
    });

    const dialog = container.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.getAttribute("aria-modal")).toBe("true");
    expect(dialog?.getAttribute("aria-labelledby")).toBe("add-notification-channel-title");

    await chooseChannel("ntfy");
    expect(input("password").type).toBe("password");
    expect(input("token").type).toBe("password");
    expect(input("token").autocomplete).toBe("new-password");
  });

  it("masks Gotify application tokens", async () => {
    await act(async () => {
      root.render(
        <AddChannelDialog
          open
          allowInsecureSMTP={false}
          onClose={() => undefined}
          onConfirm={async () => undefined}
        />,
      );
    });
    await chooseChannel("gotify");

    expect(input("app_token").type).toBe("password");
    expect(input("app_token").autocomplete).toBe("new-password");
  });

  it("submits a deliverable email channel with required recipients and secure transport", async () => {
    const onConfirm = vi.fn(async (_payload: Record<string, unknown>) => undefined);

    await act(async () => {
      root.render(
        <AddChannelDialog
          open
          allowInsecureSMTP={false}
          onClose={() => undefined}
          onConfirm={onConfirm}
        />,
      );
    });
    await chooseEmailChannel();

    expect([...select("smtp_tls_mode").options].map((option) => option.value)).toEqual(["starttls", "implicit"]);
    await fillRequiredEmailFields();
    await setValue(input("smtp_user"), "alerts@example.test");
    await setValue(input("smtp_pass"), "new-password");
    await click(button("save"));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledWith({
      name: "Operations email",
      type: "email",
      enabled: true,
      config: {
        smtp_host: "smtp.example.test",
        smtp_port: 587,
        smtp_tls_mode: "starttls",
        smtp_user: "alerts@example.test",
        smtp_pass: "new-password",
        from: "alerts@example.test",
        to: "ops@example.test, oncall@example.test",
        allow_insecure_smtp: false,
      },
    });
  });

  it("requires a typed acknowledgement before submitting policy-enabled insecure SMTP", async () => {
    const onConfirm = vi.fn(async (_payload: Record<string, unknown>) => undefined);

    await act(async () => {
      root.render(
        <AddChannelDialog
          open
          allowInsecureSMTP
          onClose={() => undefined}
          onConfirm={onConfirm}
        />,
      );
    });
    await chooseEmailChannel();
    await fillRequiredEmailFields();
    await setValue(input("smtp_port"), "25");
    await setValue(select("smtp_tls_mode"), "insecure");

    expect(input("smtp_insecure_acknowledgement")).toBeTruthy();
    await click(button("save"));
    expect(onConfirm).not.toHaveBeenCalled();
    expect(container.textContent).toContain("errors.smtpInsecureAckRequired");

    await setValue(input("smtp_insecure_acknowledgement"), SMTP_INSECURE_ACKNOWLEDGEMENT);
    await click(button("save"));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    const payload = onConfirm.mock.calls[0]![0];
    expect(payload).toMatchObject({
      name: "Operations email",
      type: "email",
      enabled: true,
      config: {
        smtp_host: "smtp.example.test",
        smtp_port: 25,
        smtp_tls_mode: "insecure",
        from: "alerts@example.test",
        to: "ops@example.test, oncall@example.test",
        allow_insecure_smtp: true,
      },
    });
    expect(JSON.stringify(payload)).not.toContain(SMTP_INSECURE_ACKNOWLEDGEMENT);
    expect(payload.config).not.toHaveProperty("smtp_user");
    expect(payload.config).not.toHaveProperty("smtp_pass");
  });

  it("edits a channel without erasing its omitted stored password", async () => {
    const onConfirm = vi.fn(async (_id: string, _payload: Record<string, unknown>) => undefined);

    await act(async () => {
      root.render(
        <EditChannelDialog
          channel={{
            id: "channel-email-1",
            name: "Operations email",
            type: "email",
            enabled: true,
            created_at: "2026-07-14T00:00:00Z",
            updated_at: "2026-07-14T00:00:00Z",
            config: {
              smtp_host: "smtp.example.test",
              smtp_port: 587,
              smtp_tls_mode: "starttls",
              smtp_user: "alerts@example.test",
              from: "alerts@example.test",
              to: "ops@example.test",
            },
          }}
          allowInsecureSMTP={false}
          onClose={() => undefined}
          onConfirm={onConfirm}
        />,
      );
    });
    await click(button("save"));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    const [, payload] = onConfirm.mock.calls[0]!;
    expect(payload).toMatchObject({
      name: "Operations email",
      config: {
        smtp_host: "smtp.example.test",
        smtp_port: 587,
        smtp_tls_mode: "starttls",
        smtp_user: "alerts@example.test",
        from: "alerts@example.test",
        to: "ops@example.test",
        allow_insecure_smtp: false,
      },
    });
    expect(payload.config).not.toHaveProperty("smtp_pass");
  });

  it("edits a redacted Slack channel without requiring or re-submitting its webhook", async () => {
    const onConfirm = vi.fn(async (_id: string, _payload: Record<string, unknown>) => undefined);

    await act(async () => {
      root.render(
        <EditChannelDialog
          channel={{
            id: "channel-slack-1",
            name: "Operations Slack",
            type: "slack",
            enabled: true,
            created_at: "2026-07-14T00:00:00Z",
            updated_at: "2026-07-14T00:00:00Z",
            config: { channel: "operations" },
          }}
          allowInsecureSMTP={false}
          onClose={() => undefined}
          onConfirm={onConfirm}
        />,
      );
    });

    expect(input("webhook_url").value).toBe("");
    await click(button("save"));

    expect(onConfirm).toHaveBeenCalledWith("channel-slack-1", {
      name: "Operations Slack",
      config: { channel: "operations" },
    });
  });
});
