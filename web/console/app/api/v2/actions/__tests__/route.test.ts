import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.test", agent: "https://agent.example.test" })),
  upstreamErrorPayload: vi.fn((status: number, payload: unknown, fallback: string) => {
    if (status >= 500) return { error: fallback };
    const record = payload && typeof payload === "object" ? payload as Record<string, unknown> : {};
    const error = typeof record.error === "string" ? record.error : fallback;
    const message = typeof record.message === "string" ? record.message : undefined;
    return message ? { error, message } : { error };
  }),
}));

vi.mock("../../../../../lib/proxyAuth", () => ({
  isMutationRequestOriginAllowed: vi.fn(() => true),
}));

import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import { resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { GET as getCollection, POST as createAction } from "../route";
import { DELETE as deleteAction, GET as getAction } from "../[id]/route";
import { POST as runAction } from "../[id]/run/route";
import { maxSavedActionEmptyMutationBodyBytes, maxSavedActionRequestBodyBytes } from "../proxy";

describe("saved-action console proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(isMutationRequestOriginAllowed).mockReturnValue(true);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("sanitizes backend failures and marks every response no-store", async () => {
    fetchMock.mockRejectedValueOnce(new Error("connect ECONNREFUSED https://secret.internal:8443"));
    const response = await getCollection(new Request("https://console.example.test/api/v2/actions"));

    expect(response.status).toBe(502);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toEqual({ error: "saved actions endpoint unavailable" });
  });

  it("turns backend configuration resolution failures into a no-store 502", async () => {
    vi.mocked(resolvedBackendBaseURLs).mockRejectedValueOnce(new Error("secret backend configuration"));
    const response = await getCollection(new Request("https://console.example.test/api/v2/actions"));

    expect(response.status).toBe(502);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(await response.json()).toEqual({ error: "saved actions endpoint unavailable" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects a cross-origin create before reading or forwarding it", async () => {
    vi.mocked(isMutationRequestOriginAllowed).mockReturnValueOnce(false);
    const response = await createAction(new Request("https://console.example.test/api/v2/actions", {
      method: "POST",
      body: JSON.stringify({ name: "blocked" }),
    }));

    expect(response.status).toBe(403);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects malformed and oversized create bodies without contacting the backend", async () => {
    const malformed = await createAction(new Request("https://console.example.test/api/v2/actions", {
      method: "POST",
      body: "not-json",
    }));
    expect(malformed.status).toBe(400);

    const oversized = await createAction(new Request("https://console.example.test/api/v2/actions", {
      method: "POST",
      body: JSON.stringify({ name: "x".repeat(maxSavedActionRequestBodyBytes) }),
    }));
    expect(oversized.status).toBe(413);
    expect(oversized.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("forwards collection create and item read with session auth", async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { id: "act-1" } }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { id: "act-1" } }), { status: 200 }));

    const createResponse = await createAction(new Request("https://console.example.test/api/v2/actions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "routine", steps: [{ name: "check", command: "uptime", target: "asset-1" }] }),
    }));
    const getResponse = await getAction(
      new Request("https://console.example.test/api/v2/actions/act-1"),
      { params: Promise.resolve({ id: "act-1" }) },
    );

    expect(createResponse.status).toBe(201);
    expect(getResponse.status).toBe(200);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.test/api/v2/actions");
    expect(fetchMock.mock.calls[0]?.[1]?.headers).toEqual({
      Cookie: "labtether_session=test",
      "Content-Type": "application/json",
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("https://api.example.test/api/v2/actions/act-1");
    expect(getResponse.headers.get("cache-control")).toContain("no-store");
  });

  it("rejects malformed item identifiers without issuing a backend request", async () => {
    const response = await deleteAction(
      new Request("https://console.example.test/api/v2/actions/bad", { method: "DELETE" }),
      { params: Promise.resolve({ id: "bad/id" }) },
    );
    expect(response.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("bounds the run body and forwards an empty same-origin run", async () => {
    const oversized = await runAction(
      new Request("https://console.example.test/api/v2/actions/act-1/run", {
        method: "POST",
        body: "x".repeat(maxSavedActionEmptyMutationBodyBytes + 1),
      }),
      { params: Promise.resolve({ id: "act-1" }) },
    );
    expect(oversized.status).toBe(413);
    expect(fetchMock).not.toHaveBeenCalled();

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ data: { action_id: "act-1", steps: [] } }), { status: 200 }));
    const response = await runAction(
      new Request("https://console.example.test/api/v2/actions/act-1/run", { method: "POST" }),
      { params: Promise.resolve({ id: "act-1" }) },
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toContain("no-store");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://api.example.test/api/v2/actions/act-1/run");
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("POST");
  });

  it("does not pass internal upstream messages through a server error", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      error: "internal",
      message: "dial postgres at secret.internal:5432",
    }), { status: 500 }));
    const response = await getAction(
      new Request("https://console.example.test/api/v2/actions/act-1"),
      { params: Promise.resolve({ id: "act-1" }) },
    );
    expect(response.status).toBe(500);
    expect(await response.json()).toEqual({ error: "failed to load saved action" });
  });
});
