import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import messages from "../../../../../messages/en/files.json";
import { ConnectionForm } from "../ConnectionForm";
import { updateFileConnection, type FileConnection } from "../fileConnectionsClient";

vi.mock("../fileConnectionsClient", async (original) => ({
  ...await original<typeof import("../fileConnectionsClient")>(),
  updateFileConnection: vi.fn(async () => ({})),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;

beforeEach(() => {
  vi.clearAllMocks();
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

async function mount(extra_config: Record<string, unknown>, transport_notice?: string) {
  const connection = { id: "webdav-test", name: "Documents", host: "files.example.test", port: 8443, protocol: "webdav", extra_config, transport_notice } as FileConnection;
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" messages={{ files: messages }}>
      <ConnectionForm protocol="webdav" existingConnection={connection} onConnect={() => {}} onCancel={() => {}} />
    </NextIntlClientProvider>,
  ));
}

function checkbox(label: string): HTMLInputElement {
  const element = [...container.querySelectorAll("label")].find((node) => node.textContent?.includes(label))?.querySelector("input");
  if (!element) throw new Error(`missing checkbox: ${label}`);
  return element;
}

async function save() {
  const button = [...container.querySelectorAll("button")].find((node) => node.textContent?.includes("Save"));
  if (!button) throw new Error("missing save button");
  await act(async () => button.click());
}

describe("WebDAV transport choices", () => {
  it("defaults to HTTPS, requires explicit HTTP acknowledgement and sends it to Hub", async () => {
    await mount({});
    expect(checkbox("Use HTTPS").checked).toBe(true);
    await act(async () => checkbox("Use HTTPS").click());
    await save();
    expect(updateFileConnection).not.toHaveBeenCalled();
    expect(container.textContent).toContain(messages.webdavCleartextRequired);
    await act(async () => checkbox(messages.webdavCleartextWarning).click());
    await save();
    expect(updateFileConnection).toHaveBeenCalledWith("webdav-test", expect.objectContaining({
      extra_config: { webdav_tls: false, webdav_allow_cleartext: true },
    }));
  });

  it("explains why a legacy plain-HTTP connection stopped opening", async () => {
    await mount({});
    expect(container.textContent).not.toContain(messages.webdavTransportNotice);
    await mount({}, "plain HTTP is no longer allowed by default");
    expect(container.querySelector('[role="note"]')?.textContent).toBe(messages.webdavTransportNotice);
  });

  it("clears a saved HTTP acknowledgement when HTTPS is enabled", async () => {
    await mount({ webdav_tls: false, webdav_allow_cleartext: true });
    expect(checkbox(messages.webdavCleartextWarning).checked).toBe(true);
    await act(async () => checkbox("Use HTTPS").click());
    await save();
    expect(updateFileConnection).toHaveBeenCalledWith("webdav-test", expect.objectContaining({
      extra_config: { webdav_tls: true },
    }));
    vi.mocked(updateFileConnection).mockClear();
    await act(async () => checkbox("Use HTTPS").click());
    expect(checkbox(messages.webdavCleartextWarning).checked).toBe(false);
    await save();
    expect(updateFileConnection).not.toHaveBeenCalled();
  });
});
