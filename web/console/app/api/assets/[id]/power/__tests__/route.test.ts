import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.com", agent: "https://agent.example.com" })),
}));

import { POST } from "../route";

const context = { params: Promise.resolve({ id: "win box" }) };

function powerRequest(action: unknown): Request {
  return new Request("https://console.example.com/api/assets/win%20box/power", {
    method: "POST",
    headers: { cookie: "labtether_session=test", origin: "https://console.example.com", "content-type": "application/json" },
    body: JSON.stringify({ action }),
  });
}

// The device menu's Restart and Shut down sent the words "restart" and "shutdown" as shell commands, which
// the Hub refused ("command not in allowlist"); they must reach the Hub's power routes.
describe("device power proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () => new Response(JSON.stringify({ status: "rebooting" }), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("sends reboot and shutdown to the Hub's power routes", async () => {
    for (const action of ["reboot", "shutdown"]) {
      const response = await POST(powerRequest(action), context);
      expect(response.status).toBe(202);
      expect(fetchMock).toHaveBeenLastCalledWith(`https://api.example.com/api/v2/assets/win%20box/${action}`,
        expect.objectContaining({ method: "POST", headers: { Cookie: "labtether_session=test" } }));
    }
  });

  it("refuses anything else without calling the Hub", async () => {
    const response = await POST(powerRequest("restart; rm -rf /"), context);
    expect(response.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
