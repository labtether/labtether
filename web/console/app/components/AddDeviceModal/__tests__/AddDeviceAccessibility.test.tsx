import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../AgentSetupStep", () => ({ AgentSetupStep: () => null }));
vi.mock("../ProxmoxSetupStep", () => ({ ProxmoxSetupStep: () => null }));
vi.mock("../PBSSetupStep", () => ({ PBSSetupStep: () => null }));
vi.mock("../DockerSetupStep", () => ({ DockerSetupStep: () => null }));
vi.mock("../PortainerSetupStep", () => ({ PortainerSetupStep: () => null }));
vi.mock("../TrueNASSetupStep", () => ({ TrueNASSetupStep: () => null }));
vi.mock("../HomeAssistantSetupStep", () => ({ HomeAssistantSetupStep: () => null }));
vi.mock("../ManualDeviceSetupStep", () => ({ ManualDeviceSetupStep: () => null }));

import { AddDeviceModal } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ compatible: [] }),
  }));
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("AddDeviceModal accessibility", () => {
  it("announces the product task and exposes a named close control", async () => {
    await act(async () => {
      root.render(<AddDeviceModal open onClose={vi.fn()} />);
    });

    const dialog = document.body.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog).not.toBeNull();

    const titleID = dialog?.getAttribute("aria-labelledby");
    const descriptionID = dialog?.getAttribute("aria-describedby");
    expect(titleID).toBeTruthy();
    expect(descriptionID).toBeTruthy();
    expect(document.getElementById(titleID ?? "")?.textContent).toBe("Add Device");
    expect(document.getElementById(descriptionID ?? "")?.textContent).toContain("Choose a connection source");
    expect(document.body.querySelector('button[aria-label="Close Add Device dialog"]')).not.toBeNull();
  });

  it("returns focus to the external opener when the dialog closes", async () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>Open Add Device</button>
          <AddDeviceModal open={open} onClose={() => setOpen(false)} />
        </>
      );
    }

    await act(async () => root.render(<Harness />));
    const opener = container.querySelector<HTMLButtonElement>("button");
    expect(opener).not.toBeNull();

    await act(async () => {
      opener?.focus();
      opener?.click();
    });
    const close = document.body.querySelector<HTMLButtonElement>('button[aria-label="Close Add Device dialog"]');
    expect(close).not.toBeNull();

    await act(async () => close?.click());

    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
