import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { checkRateLimit, pruneExpired } from "../../../lib/rateLimit";

describe("checkRateLimit", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // Reset module state by advancing time past any existing windows
    // and pruning, ensuring a clean slate
    vi.setSystemTime(Date.now() + 60 * 60 * 1000);
    pruneExpired();
    vi.setSystemTime(0);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("allows the first 10 requests", () => {
    for (let i = 0; i < 10; i++) {
      const result = checkRateLimit("test-key-allow");
      expect(result.success).toBe(true);
      expect(result.remaining).toBe(9 - i);
    }
  });

  it("blocks the 11th request", () => {
    for (let i = 0; i < 10; i++) {
      checkRateLimit("test-key-block");
    }
    const result = checkRateLimit("test-key-block");
    expect(result.success).toBe(false);
    expect(result.remaining).toBe(0);
  });

  it("allows requests again after window expires", () => {
    for (let i = 0; i < 10; i++) {
      checkRateLimit("test-key-expire");
    }
    const blocked = checkRateLimit("test-key-expire");
    expect(blocked.success).toBe(false);

    // Advance past the 15-minute window
    vi.advanceTimersByTime(15 * 60 * 1000 + 1);

    const result = checkRateLimit("test-key-expire");
    expect(result.success).toBe(true);
    expect(result.remaining).toBe(9);
  });

  it("tracks different keys separately", () => {
    for (let i = 0; i < 10; i++) {
      checkRateLimit("key-a");
    }
    const blockedA = checkRateLimit("key-a");
    expect(blockedA.success).toBe(false);

    const allowedB = checkRateLimit("key-b");
    expect(allowedB.success).toBe(true);
    expect(allowedB.remaining).toBe(9);
  });

  it("pruneExpired removes expired entries", () => {
    checkRateLimit("prune-key");
    // Advance past the window
    vi.advanceTimersByTime(15 * 60 * 1000 + 1);
    pruneExpired();

    // After pruning, the key should be gone and a new request starts fresh
    const result = checkRateLimit("prune-key");
    expect(result.success).toBe(true);
    expect(result.remaining).toBe(9);
  });
});
