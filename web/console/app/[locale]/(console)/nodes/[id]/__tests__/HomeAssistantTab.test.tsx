import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Asset } from "../../../../../console/models";

const mocks = vi.hoisted(() => ({
  fetchStatus: vi.fn<() => Promise<void>>(),
  startCollectorRun: vi.fn<(collectorID: string) => Promise<number>>(),
  status: { assets: [] as unknown[] },
  waitForCollectorRun: vi.fn(),
}));

vi.mock("../../../../../contexts/StatusContext", () => ({
  useFastStatus: () => mocks.status,
  useStatusControls: () => ({ fetchStatus: mocks.fetchStatus }),
}));

vi.mock("../../../../../components/AddDeviceModal/collectorSync", () => ({
  startCollectorRun: mocks.startCollectorRun,
  waitForCollectorRun: mocks.waitForCollectorRun,
}));

vi.mock("../../../../../../i18n/navigation", () => ({
  Link: ({ children, href }: { children: ReactNode; href: string }) => <a href={href}>{children}</a>,
}));

import { HomeAssistantTab } from "../HomeAssistantTab";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const hubAsset: Asset = {
  id: "ha-root",
  type: "connector-cluster",
  name: "Disposable HA",
  source: "homeassistant",
  status: "online",
  last_seen_at: "2026-07-16T00:00:00Z",
  metadata: {
    collector_id: "collector-ha",
    collector_base_url: "http://ha.invalid:8123",
    connector_type: "homeassistant",
    discovered: "7",
  },
};

const staleEntity: Asset = {
  id: "ha-entity-light-stale",
  type: "ha-entity",
  name: "Last Successful Light",
  source: "homeassistant",
  status: "online",
  last_seen_at: "2026-07-16T00:00:00Z",
  metadata: {
    collector_id: "collector-ha",
    entity_id: "light.last_success",
    domain: "light",
    state: "off",
  },
};

let container: HTMLDivElement;
let root: Root;

function refreshButton(): HTMLButtonElement {
  const button = [...container.querySelectorAll("button")].find((candidate) => candidate.textContent?.trim() === "Refresh");
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error("Refresh button not found");
  }
  return button;
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  mocks.status.assets = [hubAsset, staleEntity];
  mocks.fetchStatus.mockReset().mockResolvedValue(undefined);
  mocks.startCollectorRun.mockReset().mockResolvedValue(1000);
  mocks.waitForCollectorRun.mockReset();
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("HomeAssistantTab manual refresh", () => {
  it("surfaces an actionable reachability error and labels retained inventory as stale", async () => {
    mocks.waitForCollectorRun.mockResolvedValue({
      ok: false,
      error: "home assistant discovery failed: dial tcp 192.0.2.10:8123: connect: connection refused",
    });

    await act(async () => {
      root.render(<HomeAssistantTab asset={hubAsset} />);
    });
    expect(container.textContent).toContain("Last Successful Light");

    await act(async () => {
      refreshButton().click();
      await new Promise<void>((resolve) => window.setTimeout(resolve, 0));
    });

    expect(mocks.startCollectorRun).toHaveBeenCalledWith("collector-ha");
    expect(mocks.fetchStatus).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("Unable to reach Home Assistant");
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("Showing data from the last successful sync");
    expect(container.textContent).toContain("Last Successful Light");
    expect(container.textContent).not.toContain("Home Assistant data refreshed from the connector");
  });

  it("claims fresh data only after the collector reports a successful pass", async () => {
    mocks.waitForCollectorRun.mockResolvedValue({ ok: true, discovered: 1 });

    await act(async () => {
      root.render(<HomeAssistantTab asset={hubAsset} />);
    });
    await act(async () => {
      refreshButton().click();
      await new Promise<void>((resolve) => window.setTimeout(resolve, 0));
    });

    expect(mocks.fetchStatus).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[role="status"]')?.textContent).toContain(
      "Home Assistant data refreshed from the connector",
    );
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });
});
