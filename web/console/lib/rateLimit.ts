interface RateLimitEntry {
  count: number;
  resetAt: number;
}

const store = new Map<string, RateLimitEntry>();

export function checkRateLimit(
  key: string,
  maxAttempts: number = 10,
  windowMs: number = 15 * 60 * 1000
): { success: boolean; remaining: number; resetAt: number } {
  const now = Date.now();
  const entry = store.get(key);

  if (entry && now > entry.resetAt) {
    store.delete(key);
  }

  const current = store.get(key);
  if (!current) {
    store.set(key, { count: 1, resetAt: now + windowMs });
    return { success: true, remaining: maxAttempts - 1, resetAt: now + windowMs };
  }

  if (current.count >= maxAttempts) {
    return { success: false, remaining: 0, resetAt: current.resetAt };
  }

  current.count++;
  return { success: true, remaining: maxAttempts - current.count, resetAt: current.resetAt };
}

export function pruneExpired(): void {
  const now = Date.now();
  for (const [key, entry] of store) {
    if (now > entry.resetAt) store.delete(key);
  }
}

// Prune expired entries every 60 seconds to prevent memory growth
if (typeof setInterval !== "undefined") {
  setInterval(pruneExpired, 60_000);
}
