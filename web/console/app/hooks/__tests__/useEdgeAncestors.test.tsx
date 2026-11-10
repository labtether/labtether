import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useEdgeAncestors } from "../useEdges";

// Risk: the device page never shows the host above a VM or container, because the console reads another field
// than the one the Hub sends (internal/hubapi/resources/edge_handlers.go HandleEdgeAncestors: id, depth, nodes).
let latest: ReturnType<typeof useEdgeAncestors>;
let root: Root;
let container: HTMLDivElement;

function Probe() {
  latest = useEdgeAncestors("vm-101");
  return null;
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

describe("useEdgeAncestors", () => {
  it("reads the hosts a device sits in from the Hub's answer", async () => {
    const answer = { id: "vm-101", depth: 10, nodes: [{ asset_id: "pve01", depth: 1 }, { asset_id: "cluster-a", depth: 2 }] };
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(answer), { status: 200, headers: { "content-type": "application/json" } })));

    await act(async () => root.render(<Probe />));

    expect(latest.ancestors.map((node) => node.asset_id)).toEqual(["pve01", "cluster-a"]);
  });
});
