import { readFileSync } from "node:fs";
import { join } from "node:path";
import { hasServiceModeAuthHeader, isTrustedServiceProxyPath } from "./proxyAuth";

export function envOrDefault(key: string, fallback: string): string {
  const value = process.env[key];
  if (!value || value.trim() === "") {
    return fallback;
  }
  return value;
}

export function backendBaseURLs() {
  const api = envOrDefault("LABTETHER_API_BASE_URL", "http://localhost:8080");
  return {
    api,
    agent: envOrDefault("LABTETHER_AGENT_BASE_URL", "http://localhost:8090")
  };
}

export type BackendBaseURLs = ReturnType<typeof backendBaseURLs>;

const routingOverrideMap = {
  "routing.api_base_url": "api",
  "routing.agent_base_url": "agent"
} as const;

type RuntimeRoutingOverrideKey = keyof typeof routingOverrideMap;

const overrideURLSchemes = new Set(["http:", "https:"]);
const controlCharsPattern = /[\u0000-\u001f\u007f]+/g;

function normalizeHost(hostname: string): string {
  return hostname
    .trim()
    .toLowerCase()
    .replace(/^\[|\]$/g, "")
    .replace(/\.+$/, "");
}

function isLoopbackHost(hostname: string): boolean {
  const normalized = normalizeHost(hostname);
  return normalized === "localhost" || normalized === "127.0.0.1" || normalized === "::1";
}

function hostsEquivalent(left: string, right: string): boolean {
  const a = normalizeHost(left);
  const b = normalizeHost(right);
  if (a === b) {
    return true;
  }
  return isLoopbackHost(a) && isLoopbackHost(b);
}

function parseAllowedOverrideOrigins(raw: string): Set<string> {
  const out = new Set<string>();
  for (const token of raw.split(",")) {
    const trimmed = token.trim();
    if (!trimmed) {
      continue;
    }
    try {
      const parsed = new URL(trimmed);
      if (overrideURLSchemes.has(parsed.protocol)) {
        out.add(normalizeURL(parsed));
      }
    } catch {
      // Ignore malformed allowlist entries.
    }
  }
  return out;
}

function normalizeURL(url: URL): string {
  return `${url.protocol}//${url.host}`;
}

function parseRuntimeOverrideURL(raw: string): URL | null {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return null;
  }
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return null;
  }
  if (!overrideURLSchemes.has(parsed.protocol)) {
    return null;
  }
  if (parsed.username || parsed.password) {
    return null;
  }
  if (parsed.search || parsed.hash) {
    return null;
  }
  if (parsed.pathname !== "" && parsed.pathname !== "/") {
    return null;
  }
  const hostname = normalizeHost(parsed.hostname);
  if (!hostname) {
    return null;
  }
  if (hostname === "0.0.0.0" || hostname.startsWith("169.254.")) {
    return null;
  }
  return parsed;
}

export type RoutingOverrideValidationResult =
  | { valid: true; normalized: string }
  | { valid: false; reason: string };

export function isRoutingOverrideKey(key: string): key is RuntimeRoutingOverrideKey {
  return key in routingOverrideMap;
}

export function validateRoutingOverrideURL(
  key: RuntimeRoutingOverrideKey,
  rawValue: string,
  base: BackendBaseURLs = backendBaseURLs(),
): RoutingOverrideValidationResult {
  const parsed = parseRuntimeOverrideURL(rawValue);
  if (!parsed) {
    return { valid: false, reason: "must be an absolute http(s) URL without credentials, query, hash, or path" };
  }

  const targetField = routingOverrideMap[key];
  let baseline: URL;
  try {
    baseline = new URL(base[targetField]);
  } catch {
    return { valid: false, reason: "baseline routing URL is invalid" };
  }

  const normalizedParsed = normalizeURL(parsed);
  const normalizedBaseline = normalizeURL(baseline);
  if (normalizedParsed === normalizedBaseline) {
    return { valid: true, normalized: normalizedParsed };
  }

  const allowedOrigins = parseAllowedOverrideOrigins(
    process.env.LABTETHER_RUNTIME_OVERRIDE_ALLOWED_ORIGINS?.trim() ?? "",
  );
  if (allowedOrigins.has(normalizedParsed)) {
    return { valid: true, normalized: normalizedParsed };
  }
  if (hostsEquivalent(parsed.hostname, baseline.hostname)) {
    return { valid: false, reason: "origin must exactly match the configured routing origin" };
  }
  return { valid: false, reason: "origin must match the configured routing origin or an explicit allowlist entry" };
}

function trimSafeError(value: string): string {
  return value.replace(controlCharsPattern, " ").replace(/\s+/g, " ").trim();
}

function extractErrorCandidate(payload: unknown): string {
  if (typeof payload === "string") {
    return payload;
  }
  if (!payload || typeof payload !== "object") {
    return "";
  }
  const record = payload as Record<string, unknown>;
  for (const field of ["error", "message", "detail"]) {
    const value = record[field];
    if (typeof value === "string" && value.trim() !== "") {
      return value;
    }
  }
  return "";
}

export function extractSafeUpstreamError(payload: unknown, fallback: string): string {
  const candidate = trimSafeError(extractErrorCandidate(payload));
  if (!candidate) {
    return fallback;
  }
  return candidate.slice(0, 240);
}

export function upstreamErrorPayload(status: number, payload: unknown, fallback: string): { error: string } {
  if (status >= 500) {
    return { error: fallback };
  }
  return { error: extractSafeUpstreamError(payload, fallback) };
}

/**
 * Infer whether browser-facing websocket URLs should use wss://.
 *
 * This must follow the frontend request protocol (direct or proxied) so
 * browser WS traffic remains same-origin with the console host/cert.
 */
export function shouldUseSecureWebSocket(request: Request): boolean {
  const forwardedProto = request.headers.get("x-forwarded-proto")?.split(",")[0]?.trim().toLowerCase() ?? "";
  if (forwardedProto === "https") {
    return true;
  }

  try {
    const reqURL = new URL(request.url);
    if (reqURL.protocol === "https:") {
      return true;
    }
  } catch {
    // ignore malformed request URL and continue heuristic checks.
  }

  return false;
}

function isTrustedURL(raw: string): boolean {
  try {
    const u = new URL(raw);
    if (u.protocol !== "http:" && u.protocol !== "https:") return false;
    const host = u.hostname;
    // Block cloud metadata / link-local (169.254.x.x)
    if (host.startsWith("169.254.")) return false;
    // Block the unspecified address
    if (host === "0.0.0.0") return false;
    return true;
  } catch {
    return false;
  }
}

let _cachedURLs: BackendBaseURLs | null = null;
let _cacheExpiry = 0;
let _cachedServiceToken = "";
let _serviceTokenCacheExpiry = 0;

type PersistedInstallSecrets = {
  api_token?: string;
  owner_token?: string;
};

function serviceTokenFilePath(): string {
  return envOrDefault("LABTETHER_API_TOKEN_FILE", "/run/labtether/api-token");
}

function readServiceToken(): string {
  const now = Date.now();
  if (now < _serviceTokenCacheExpiry) {
    return _cachedServiceToken;
  }

  try {
    _cachedServiceToken = readFileSync(serviceTokenFilePath(), "utf8").trim();
  } catch {
    try {
      const raw = readFileSync(join(envOrDefault("LABTETHER_INSTALL_STATE_DIR", "/labtether-data/install"), "secrets.json"), "utf8");
      const parsed = JSON.parse(raw) as PersistedInstallSecrets;
      _cachedServiceToken = parsed.api_token?.trim() || "";
    } catch {
      _cachedServiceToken = "";
    }
  }

  _serviceTokenCacheExpiry = now + 5_000;
  return _cachedServiceToken;
}

/** Invalidate the resolved URL cache (e.g. after saving routing settings). */
export function invalidateBackendURLCache() {
  _cachedURLs = null;
  _cacheExpiry = 0;
}

export async function resolvedBackendBaseURLs(): Promise<BackendBaseURLs> {
  const now = Date.now();
  if (_cachedURLs && now < _cacheExpiry) {
    return { ..._cachedURLs }; // return a copy to prevent mutation
  }

  const base = backendBaseURLs();
  const baseline = { ...base };
  const authHeaders = backendAuthHeaders();

  const payload = await getJSON<{ overrides?: Record<string, string> }>(`${base.api}/settings/runtime`, authHeaders);
  const overrides = payload?.overrides ?? {};

  for (const [settingKey, field] of Object.entries(routingOverrideMap) as Array<[RuntimeRoutingOverrideKey, keyof BackendBaseURLs]>) {
    const overrideValue = overrides[settingKey];
    if (typeof overrideValue !== "string" || overrideValue.trim() === "") {
      continue;
    }
    const validation = validateRoutingOverrideURL(settingKey, overrideValue, baseline);
    if (validation.valid) {
      base[field] = validation.normalized;
    }
  }

  _cachedURLs = { ...base }; // store a copy
  _cacheExpiry = now + 30_000;
  return base;
}

export function backendAuthHeaders(): HeadersInit {
  const token = process.env.LABTETHER_API_TOKEN?.trim() || readServiceToken();
  if (!token) {
    return {};
  }

  return {
    Authorization: `Bearer ${token}`
  };
}

export function backendAuthHeadersWithCookie(request: Request): Record<string, string> {
  const headers: Record<string, string> = {};

  const pathname = new URL(request.url).pathname;
  const cookie = request.headers.get("cookie");
  if (cookie) {
    headers["Cookie"] = cookie;
  }
  const allowServiceAuth = isTrustedServiceProxyPath(pathname) && hasServiceModeAuthHeader(request.headers);

  // Never inject LABTETHER_API_TOKEN here; per-request proxy auth must come
  // from caller credentials only (session cookie), except trusted service-mode
  // routes explicitly allowlisted for token/bearer usage.
  const authorization = request.headers.get("authorization");
  if (allowServiceAuth && authorization) {
    headers["Authorization"] = authorization;
  }
  const tokenHeader = request.headers.get("x-labtether-token");
  if (allowServiceAuth && tokenHeader) {
    headers["X-Labtether-Token"] = tokenHeader;
  }

  return headers;
}

export type ProbeResult = {
  name: string;
  url: string;
  ok: boolean;
  status: "up" | "down";
  code?: number;
  latencyMs: number;
  error?: string;
};

export async function probeEndpoint(name: string, url: string, headers?: HeadersInit): Promise<ProbeResult> {
  const started = Date.now();
  try {
    const response = await fetch(url, {
      cache: "no-store",
      headers
    });

    return {
      name,
      url,
      ok: response.ok,
      status: response.ok ? "up" : "down",
      code: response.status,
      latencyMs: Date.now() - started
    };
  } catch (error) {
    return {
      name,
      url,
      ok: false,
      status: "down",
      latencyMs: Date.now() - started,
      error: error instanceof Error ? error.message : "endpoint unreachable"
    };
  }
}

export async function getJSON<T>(url: string, headers?: HeadersInit, timeoutMs = 10000): Promise<T | null> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, {
      cache: "no-store",
      headers,
      signal: controller.signal,
    });
    if (!response.ok) {
      return null;
    }
    return (await response.json()) as T;
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}
