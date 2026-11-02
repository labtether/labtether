import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useIncidents } from "../useIncidents";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// Risk: a Hub that refuses the issue list (500) left the list empty with no error, so the Issues page said
// "No active issues" while issues could exist (found on the 2.0 lab Hub with a refused answer, 2026-11-02).
let container: HTMLDivElement;
let root: Root;
let seen: { error: string | null; loading: boolean; count: number } | null;

function Probe() {
  const { incidents, loading, error } = useIncidents();
  seen = { error, loading, count: incidents.length };
  return null;
}

beforeEach(() => {
  container = document.createElement("div");
  root = createRoot(container);
  seen = null;
});

afterEach(() => {
  act(() => root.unmount());
  vi.unstubAllGlobals();
});

it("reports a refused issue list as an error, not as no issues", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: "unavailable" }, { status: 500 })));
  await act(async () => root.render(<Probe />));
  expect(seen).toMatchObject({ loading: false, count: 0 });
  expect(seen?.error).toBeTruthy();
});

it("lists the issues the Hub returns", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => Response.json({ incidents: [{ id: "i-1", title: "nas01 down" }] })));
  await act(async () => root.render(<Probe />));
  expect(seen).toMatchObject({ loading: false, count: 1, error: null });
});
