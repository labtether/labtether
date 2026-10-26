import { afterEach, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=fixture" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://hub.example.test" })),
}));

import { GET } from "../route";

const event = {
  id: "event-1", incident_id: "incident/one", event_type: "alert_fired", source_ref: "alert:one",
  summary: "CPU alert fired", occurred_at: "2026-10-11T10:00:00Z", created_at: "2026-10-11T11:00:00Z",
  metadata: { asset_ids: ["asset-one"], value: 92 },
};
const params = { params: Promise.resolve({ id: "incident/one" }) };

afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks(); });

it("proxies the existing events URL to the authorized canonical timeline and forwards its page", async () => {
  const payload = { events: [event], next_cursor: "opaque/page+two" };
  const upstream = vi.fn(async () => Response.json(payload));
  vi.stubGlobal("fetch", upstream);
  const request = new Request("https://console.example.test/api/incidents/incident%2Fone/events?limit=50&cursor=opaque%2Fpage%2Bone&ignored=private");
  const response = await GET(request, params);
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual(payload);
  expect(upstream).toHaveBeenCalledExactlyOnceWith(
    "https://hub.example.test/incidents/incident%2Fone/timeline?limit=50&cursor=opaque%2Fpage%2Bone",
    { cache: "no-store", headers: { Cookie: "labtether_session=fixture" }, signal: request.signal },
  );
});

it("keeps upstream access and cursor failures as errors, never empty success", async () => {
  for (const code of [400, 403, 404, 500]) {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: "unavailable" }, { status: code })));
    const response = await GET(new Request("https://console.example.test/api/incidents/incident%2Fone/events"), params);
    expect(response.status).toBe(code);
    expect(await response.json()).toEqual({ error: "unavailable" });
  }
});

it("rejects invalid JSON, wrong wire fields and foreign incident rows instead of hiding retained history", async () => {
  for (const body of ["not JSON", "null", JSON.stringify({ events: [{ id: "old-ui-shape", title: "wrong" }] }),
    JSON.stringify({ events: [{ ...event, incident_id: "other-incident" }] })]) {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body, { status: 200 })));
    const response = await GET(new Request("https://console.example.test/api/incidents/incident%2Fone/events"), params);
    expect(response.status).toBe(502);
    expect(await response.json()).toEqual({ error: "failed to load incident timeline" });
  }
});
