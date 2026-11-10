import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAssetDependencies } from "../useAssetDependencies";

// Risk: the Hub answers the Map's links request with 404 (a device in the list was removed a moment ago) and the
// Map silently draws every device with no links, as if the Hub had lost them.
let latest: ReturnType<typeof useAssetDependencies>;
let root: Root;
let container: HTMLDivElement;

function Probe({ ids }: { ids: string[] }) {
  latest = useAssetDependencies(ids);
  return null;
}

const reply = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const edge = { id: "e1", source_asset_id: "pve01", target_asset_id: "vm-101", relationship_type: "runs_on" };

async function show(ids: string[]) {
  await act(async () => root.render(<Probe ids={ids} />));
}

beforeEach(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("useAssetDependencies", () => {
  it("keeps the links it had and reports the failure when the Hub refuses the list", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(reply({ edges: [edge] }))
      .mockResolvedValueOnce(reply({ error: "device not found" }, 404))
      .mockResolvedValueOnce(reply({ edges: [edge] }));
    vi.stubGlobal("fetch", fetchMock);

    await show(["pve01", "vm-101"]);
    expect(latest.dependencies).toHaveLength(1);
    expect(latest.relationshipsFailed).toBe(false);

    await show(["pve01", "vm-101", "vm-gone"]);
    expect(latest.relationshipsFailed).toBe(true);
    expect(latest.dependencies).toHaveLength(1);

    await act(async () => latest.retry());
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(latest.relationshipsFailed).toBe(false);
    expect(latest.dependencies).toHaveLength(1);
  });

  it("shows a device that has no links without an error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply({ edges: [] })));
    await show(["nas01"]);
    expect(latest.dependencies).toEqual([]);
    expect(latest.relationshipsFailed).toBe(false);
  });
});
