import { describe, it, expect, beforeEach, vi } from "vitest";

// Reset the module between tests so the in-memory store is fresh.
// We use dynamic import after resetting modules.
async function freshModule() {
  vi.resetModules();
  return import("../rateLimit");
}

describe("checkRateLimit", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("allows requests under the limit", async () => {
    const { checkRateLimit } = await freshModule();
    const result = checkRateLimit("test-ip-1", 5, 60_000);
    expect(result.success).toBe(true);
    expect(result.remaining).toBe(4);
  });

  it("tracks remaining attempts correctly", async () => {
    const { checkRateLimit } = await freshModule();
    for (let i = 0; i < 4; i++) {
      checkRateLimit("test-ip-2", 5, 60_000);
    }
    const result = checkRateLimit("test-ip-2", 5, 60_000);
    expect(result.success).toBe(true);
    expect(result.remaining).toBe(0);
  });

  it("blocks after limit is reached", async () => {
    const { checkRateLimit } = await freshModule();
    for (let i = 0; i < 10; i++) {
      checkRateLimit("test-ip-3", 10, 60_000);
    }
    const blocked = checkRateLimit("test-ip-3", 10, 60_000);
    expect(blocked.success).toBe(false);
    expect(blocked.remaining).toBe(0);
  });

  it("returns resetAt timestamp in the future", async () => {
    const { checkRateLimit } = await freshModule();
    const now = Date.now();
    const result = checkRateLimit("test-ip-4", 10, 60_000);
    expect(result.resetAt).toBeGreaterThanOrEqual(now);
    expect(result.resetAt).toBeLessThanOrEqual(now + 61_000);
  });

  it("resets after the window expires", async () => {
    const { checkRateLimit } = await freshModule();
    // Use a very short window
    for (let i = 0; i < 3; i++) {
      checkRateLimit("test-ip-5", 3, 1); // 1ms window
    }
    const blocked = checkRateLimit("test-ip-5", 3, 1);
    expect(blocked.success).toBe(false);

    // Wait for the window to expire
    await new Promise((r) => setTimeout(r, 10));
    const afterExpiry = checkRateLimit("test-ip-5", 3, 1);
    expect(afterExpiry.success).toBe(true);
  });

  it("isolates keys from each other", async () => {
    const { checkRateLimit } = await freshModule();
    for (let i = 0; i < 5; i++) {
      checkRateLimit("ip-a", 5, 60_000);
    }
    // ip-a is exhausted
    expect(checkRateLimit("ip-a", 5, 60_000).success).toBe(false);
    // ip-b should still be fine
    expect(checkRateLimit("ip-b", 5, 60_000).success).toBe(true);
  });

  it("uses default values (10 attempts, 15 min)", async () => {
    const { checkRateLimit } = await freshModule();
    // Call with defaults
    const result = checkRateLimit("test-defaults");
    expect(result.success).toBe(true);
    expect(result.remaining).toBe(9); // 10 - 1
    // resetAt should be ~15 minutes from now
    const expectedReset = Date.now() + 15 * 60 * 1000;
    expect(result.resetAt).toBeGreaterThan(expectedReset - 2000);
    expect(result.resetAt).toBeLessThan(expectedReset + 2000);
  });
});

describe("pruneExpired", () => {
  it("removes expired entries", async () => {
    const { checkRateLimit, pruneExpired } = await freshModule();
    // Create an entry with a 1ms window
    checkRateLimit("prune-test", 5, 1);
    await new Promise((r) => setTimeout(r, 10));
    pruneExpired();
    // After pruning, a new request should get full remaining
    const result = checkRateLimit("prune-test", 5, 60_000);
    expect(result.success).toBe(true);
    expect(result.remaining).toBe(4); // fresh entry
  });
});
