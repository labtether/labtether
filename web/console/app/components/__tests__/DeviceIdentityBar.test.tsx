import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { Asset } from "../../console/models";
import { DeviceIdentityBar } from "../DeviceIdentityBar";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

function assetWithStatus(status: string): Asset {
  return {
    id: "ha-root",
    type: "connector-cluster",
    name: "Disposable HA",
    source: "homeassistant",
    status,
    last_seen_at: new Date().toISOString(),
  };
}

async function renderIdentity(status: string, freshnessStatus: "ok" | "pending" | "bad") {
  await act(async () => {
    root.render(
      <DeviceIdentityBar
        asset={assetWithStatus(status)}
        telemetry={null}
        groupName=""
        freshnessStatus={freshnessStatus}
        agentConnected={false}
        activePanel={null}
        onBack={() => {}}
      />,
    );
  });
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("DeviceIdentityBar status precedence", () => {
  it("shows Offline when explicit asset status is offline despite a fresh last-seen state", async () => {
    await renderIdentity("offline", "ok");

    expect(container.textContent).toContain("Offline");
    expect(container.textContent).not.toContain("Online");
  });

  it.each(["unknown", "unavailable"])(
    "shows Offline when explicit asset status is %s despite a fresh last-seen state",
    async (status) => {
      await renderIdentity(status, "ok");

      expect(container.textContent).toContain("Offline");
      expect(container.textContent).not.toContain("Online");
    },
  );

  it("shows Unresponsive for an explicit degraded status despite fresh last-seen state", async () => {
    await renderIdentity("degraded", "ok");

    expect(container.textContent).toContain("Unresponsive");
    expect(container.textContent).not.toContain("Online");
  });

  it("still lets stale freshness override an explicit healthy status", async () => {
    await renderIdentity("online", "pending");

    expect(container.textContent).toContain("Unresponsive");
    expect(container.textContent).not.toContain("Online");
  });
});
