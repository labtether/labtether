interface RateLimitEntry {
  count: number;
  resetAt: number;
}

const store = new Map<string, RateLimitEntry>();
export const MAX_RATE_LIMIT_ENTRIES = 10_000;

function ensureCapacity(): void {
  if (store.size < MAX_RATE_LIMIT_ENTRIES) return;
  pruneExpired();
  while (store.size >= MAX_RATE_LIMIT_ENTRIES) {
    const oldest = store.keys().next().value as string | undefined;
    if (oldest === undefined) break;
    store.delete(oldest);
  }
}

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
    ensureCapacity();
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

export function rateLimitStoreSize(): number {
  return store.size;
}

export function resetRateLimitStore(): void {
  store.clear();
}
