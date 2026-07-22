import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({
    api: "https://api.example.com",
    agent: "https://agent.example.com",
  })),
}));

import { POST as verifySnapshot } from "../snapshots/verify/route";
import {
  DELETE as forgetSnapshot,
  POST as forgetSnapshotLegacy,
} from "../snapshots/forget/route";
import {
  DELETE as forgetGroup,
  POST as forgetGroupLegacy,
} from "../groups/forget/route";

const context = { params: Promise.resolve({ assetId: "pbs server" }) };

function sameOriginRequest(url: string, init: RequestInit): Request {
  return new Request(url, {
    ...init,
    headers: {
      cookie: "labtether_session=test",
      origin: "https://console.example.com",
      ...(init.headers ?? {}),
    },
  });
}

describe("PBS mutation proxy contracts", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () => new Response(JSON.stringify({ status: "ok" }), {
      status: 200,
      headers: { "content-type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("preserves the datastore query for snapshot verification", async () => {
    const response = await verifySnapshot(
      sameOriginRequest(
        "https://console.example.com/api/pbs/assets/pbs%20server/snapshots/verify?store=qa-store",
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ store: "qa-store" }),
        },
      ),
      context,
    );

    expect(response.status).toBe(200);
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "https://api.example.com/pbs/assets/pbs%20server/snapshots/verify?store=qa-store",
    );
    expect(fetchMock.mock.calls[0]?.[1]).toEqual(expect.objectContaining({ method: "POST" }));
  });

  it("forwards current snapshot forget as DELETE with the full selector", async () => {
    const response = await forgetSnapshot(
      sameOriginRequest(
        "https://console.example.com/api/pbs/assets/pbs%20server/snapshots/forget?store=qa-store&backup-type=vm&backup-id=900&backup-time=1784001600",
        { method: "DELETE" },
      ),
      context,
    );

    expect(response.status).toBe(200);
    const [input, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(input)).toContain("/snapshots/forget?store=qa-store");
    expect(String(input)).toContain("backup-type=vm");
    expect(String(input)).toContain("backup-id=900");
    expect(String(input)).toContain("backup-time=1784001600");
    expect(init).toEqual(expect.objectContaining({ method: "DELETE" }));
  });

  it("translates the previous snapshot POST body for an already-open console", async () => {
    await forgetSnapshotLegacy(
      sameOriginRequest(
        "https://console.example.com/api/pbs/assets/pbs%20server/snapshots/forget",
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({
            store: "qa-store",
            backup_type: "vm",
            backup_id: "900",
            backup_time: 1784001600,
          }),
        },
      ),
      context,
    );

    const [input, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(input)).toContain("/snapshots/forget?store=qa-store");
    expect(String(input)).toContain("backup-time=1784001600");
    expect(init).toEqual(expect.objectContaining({ method: "DELETE" }));
  });

  it("supports current and previous group-forget requests", async () => {
    await forgetGroup(
      sameOriginRequest(
        "https://console.example.com/api/pbs/assets/pbs%20server/groups/forget?store=qa-store&backup-type=vm&backup-id=900",
        { method: "DELETE" },
      ),
      context,
    );
    await forgetGroupLegacy(
      sameOriginRequest(
        "https://console.example.com/api/pbs/assets/pbs%20server/groups/forget",
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ store: "qa-store", backup_type: "vm", backup_id: "900" }),
        },
      ),
      context,
    );

    for (const [input, init] of fetchMock.mock.calls) {
      expect(String(input)).toContain("/groups/forget?store=qa-store");
      expect(String(input)).toContain("backup-type=vm");
      expect(String(input)).toContain("backup-id=900");
      expect(init).toEqual(expect.objectContaining({ method: "DELETE" }));
    }
  });
});
