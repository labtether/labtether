import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

describe("Prometheus connection-test proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards only the stored-password reuse flag, never a hydrated secret", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ success: true }), { status: 200 }));
    const response = await POST(new Request(
      "https://console.example.com/api/settings/prometheus/test-connection",
      {
        method: "POST",
        headers: { origin: "https://console.example.com", "content-type": "application/json" },
        body: JSON.stringify({
          url: "https://metrics.example.test/api/v1/write",
          username: "alice",
          password: "",
          use_stored_password: true,
        }),
      },
    ));

    expect(response.status).toBe(200);
	expect(response.headers.get("cache-control")).toBe("no-store");
    const forwarded = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    expect(forwarded).toEqual({
      url: "https://metrics.example.test/api/v1/write",
      username: "alice",
      password: "",
      use_stored_password: true,
    });
  });

  it("redacts an explicitly supplied password from transport errors", async () => {
    fetchMock.mockRejectedValueOnce(new Error("dial failed with replacement-secret"));
    const response = await POST(new Request(
      "https://console.example.com/api/settings/prometheus/test-connection",
      {
        method: "POST",
        headers: { origin: "https://console.example.com", "content-type": "application/json" },
        body: JSON.stringify({
          url: "https://metrics.example.test/api/v1/write",
          username: "alice",
          password: "replacement-secret",
        }),
      },
    ));

    expect(response.status).toBe(502);
    expect(await response.text()).not.toContain("replacement-secret");
	expect(response.headers.get("cache-control")).toBe("no-store");
  });

	it("rejects malformed and oversized bodies before calling the backend", async () => {
		const malformed = await POST(new Request(
			"https://console.example.com/api/settings/prometheus/test-connection",
			{
				method: "POST",
				headers: { origin: "https://console.example.com", "content-type": "application/json" },
				body: "not-json",
			},
		));
		expect(malformed.status).toBe(400);
		expect(malformed.headers.get("cache-control")).toBe("no-store");

		const oversized = await POST(new Request(
			"https://console.example.com/api/settings/prometheus/test-connection",
			{
				method: "POST",
				headers: { origin: "https://console.example.com", "content-type": "application/json" },
				body: JSON.stringify({ password: "x".repeat(25 * 1024) }),
			},
		));
		expect(oversized.status).toBe(413);
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it("fails closed on a malformed successful backend response", async () => {
		fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200 }));
		const response = await POST(new Request(
			"https://console.example.com/api/settings/prometheus/test-connection",
			{
				method: "POST",
				headers: { origin: "https://console.example.com", "content-type": "application/json" },
				body: JSON.stringify({ url: "https://metrics.example.test/write" }),
			},
		));

		expect(response.status).toBe(502);
		expect(await response.json()).toEqual({ error: "invalid prometheus remote_write response" });
		expect(response.headers.get("cache-control")).toBe("no-store");
	});
});
