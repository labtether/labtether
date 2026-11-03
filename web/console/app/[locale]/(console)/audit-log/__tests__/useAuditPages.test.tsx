import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAuditPages } from "../useAuditPages";

// Risks: the owner sees only the newest rows with no way to older ones, or Export CSV leaves out rows the Hub has.

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const row = (n: number) => ({ id: `a${n}`, type: "webhook.deleted", timestamp: new Date(Date.UTC(2026, 10, 3, 0, 0, 10_000 - n)).toISOString() });
const log = Array.from({ length: 450 }, (_, i) => row(i));
let api: ReturnType<typeof useAuditPages>;
let root: Root;
let container: HTMLDivElement;

function Consumer() {
  api = useAuditPages(true);
  return null;
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    const query = new URL(url, "https://console.example.com").searchParams;
    const offset = Number(query.get("offset")), limit = Number(query.get("limit"));
    return new Response(JSON.stringify({ data: { events: log.slice(offset, offset + limit) } }));
  }));
  container = document.createElement("div");
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
});

describe("useAuditPages", () => {
  it("loads older pages on request and reads the whole log for an export", async () => {
    await act(async () => root.render(<Consumer />));
    expect(api.entries).toHaveLength(200);
    expect(api.more).toBe(true);

    await act(async () => { await api.loadMore(); });
    await act(async () => { await api.loadMore(); });
    expect(api.entries.map((e) => e.id)).toEqual(log.map((e) => e.id));
    expect(api.more).toBe(false);

    let all: Awaited<ReturnType<typeof api.readAll>> = [];
    await act(async () => { all = await api.readAll(); });
    expect(all).toHaveLength(450);
  });
});
