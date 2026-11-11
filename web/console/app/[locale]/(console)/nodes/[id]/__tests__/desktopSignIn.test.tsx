import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

import type { VNCViewerHandle } from "../../../../../components/VNCViewer";
import { useDesktopConnectionUx } from "../useDesktopConnectionUx";
import { computeAvailableDesktopProtocols } from "../useDesktopProtocolState";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let ux: ReturnType<typeof useDesktopConnectionUx> | null;
const sendCredentials = vi.fn();
const onCancel = vi.fn();
const vncRef = { current: { sendCredentials } as unknown as VNCViewerHandle };

function Harness({ autoPassword }: { autoPassword: string | null }): ReactNode {
  ux = useDesktopConnectionUx({ protocol: "vnc", vncRef, autoPassword });
  return ux.renderCredentialOverlay(onCancel);
}

async function mount(autoPassword: string | null) {
  await act(async () => {
    root.render(<Harness autoPassword={autoPassword} />);
  });
}

async function deviceAsksForPassword() {
  await act(async () => {
    ux?.handleCredentialsRequired({ types: ["password"] });
  });
}

const passwordField = () => container.querySelector('input[type="password"]');

beforeEach(() => {
  ux = null;
  vi.clearAllMocks();
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
});

// The device page has its own sign-in code. A VNC server the person runs (Windows, macOS) got the Hub's session
// password ("authentication rejected"), the person was never asked, and the sign-in card had no Cancel.
describe("device page Remote View sign-in", () => {
  it("asks for the VNC password when the Hub sends no session password, and Cancel ends the attempt", async () => {
    await mount(null);
    await deviceAsksForPassword();
    expect(sendCredentials).not.toHaveBeenCalled();
    expect(passwordField()).not.toBeNull();

    const cancel = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent === "cancel",
    );
    await act(async () => {
      cancel?.click();
    });
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("asks on the next attempt when the device refused the session password", async () => {
    await mount("session-one");
    await deviceAsksForPassword();
    expect(sendCredentials).toHaveBeenCalledExactlyOnceWith({ password: "session-one" });
    expect(ux?.takeSessionPasswordRefusal()).toBe(true);

    // The page connects again: a new ticket, a new session password.
    await mount("session-two");
    await deviceAsksForPassword();
    expect(sendCredentials).toHaveBeenCalledOnce();
    expect(passwordField()).not.toBeNull();
    expect(ux?.takeSessionPasswordRefusal()).toBe(false);
  });
});

// "RDP (Recommended)" was offered for every Windows device, also on a Hub without the RDP gateway.
describe("protocols offered for a Windows device", () => {
  const windows = { source: "agent", platform: "windows", metadata: {} };

  it("leaves RDP out when the Hub says its RDP gateway is not running", () => {
    expect(computeAvailableDesktopProtocols(windows, true, false)).toEqual(["vnc"]);
  });

  it("offers RDP first when the gateway is running or the Hub has not said", () => {
    expect(computeAvailableDesktopProtocols(windows, true, true)[0]).toBe("rdp");
    expect(computeAvailableDesktopProtocols(windows, true, null)[0]).toBe("rdp");
  });
});
