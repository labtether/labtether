import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../../lib/backend", () => ({
  backendAuthHeadersWithCookie: vi.fn(() => ({ Cookie: "labtether_session=test" })),
  resolvedBackendBaseURLs: vi.fn(async () => ({ api: "https://api.example.test" })),
}));

import { GET } from "../route";

describe("schedule list console proxy", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("forwards page two and its size to the Hub", async () => {
    const page = { data: [{ id: "schedule-101" }], meta: { total: 101, page: 2, per_page: 100 } };
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(page), { status: 200 }));

    const response = await GET(new Request("https://console.example.test/api/v2/schedules?page=2&per_page=100"));

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(page);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.test/api/v2/schedules?page=2&per_page=100",
      expect.objectContaining({ headers: { Cookie: "labtether_session=test" } }),
    );
  });

  it("rejects invalid pagination before contacting the Hub", async () => {
    const response = await GET(new Request("https://console.example.test/api/v2/schedules?page=2&per_page=101"));

    expect(response.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
