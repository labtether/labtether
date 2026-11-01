import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com" })),
}));

import { PATCH } from "../route";

// Risk: the rule switch and the rule editor both go through this PATCH. It must reach the hub with the session and
// the exact changes, give the hub's refusal back (the editor shows it), and refuse a page on another origin.
describe("alert rule console proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  const context = { params: Promise.resolve({ ruleId: "rule/101" }) };
  const patch = (body: string, origin = "https://console.example.com") =>
    PATCH(new Request("https://console.example.com/api/alerts/rules/rule", {
      method: "PATCH",
      headers: { origin, cookie: "labtether_session=test", "content-type": "application/json" },
      body,
    }), context);

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("sends the change to the rule with session auth", async () => {
    const rule = { rule: { id: "rule/101", status: "paused" } };
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(rule), { status: 200 }));

    const response = await patch(JSON.stringify({ status: "paused" }));

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(rule);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe("https://api.example.com/alerts/rules/rule%2F101");
    expect(init).toMatchObject({ method: "PATCH", headers: { Cookie: "labtether_session=test", "Content-Type": "application/json" } });
    expect(JSON.parse(init.body)).toEqual({ status: "paused" });
  });

  it("gives back the hub's refusal with its status", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ error: "name cannot be empty" }), { status: 400 }));

    const response = await patch(JSON.stringify({ name: "" }));

    expect(response.status).toBe(400);
    expect(await response.json()).toEqual({ error: "name cannot be empty" });
  });

  it("refuses another origin and a body that is not an object, without calling the hub", async () => {
    expect((await patch(JSON.stringify({ status: "paused" }), "https://evil.example.net")).status).toBe(403);
    expect((await patch("[]")).status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
