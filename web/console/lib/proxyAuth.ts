const trustedServicePathPrefixes = [
  "/api/status",
  "/api/status/live",
];

const mutatingMethods = new Set(["POST", "PUT", "PATCH", "DELETE"]);

type RequestLike = {
  method: string;
  url: string;
  headers: Headers;
};

export function hasLabtetherSessionCookie(cookieHeader: string | null): boolean {
  if (!cookieHeader) {
    return false;
  }
  return cookieHeader.split(";").some(
    (c) => c.trim().startsWith("labtether_session=")
  );
}

export function isTrustedServiceProxyPath(pathname: string): boolean {
  return trustedServicePathPrefixes.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)
  );
}

export function hasServiceModeAuthHeader(headers: Headers): boolean {
  const tokenHeader = headers.get("x-labtether-token");
  if (tokenHeader && tokenHeader.trim() !== "") {
    return true;
  }
  const authorization = headers.get("authorization");
  return Boolean(authorization && authorization.trim() !== "");
}

function firstForwardedValue(raw: string | null): string {
  if (!raw) {
    return "";
  }
  return raw.split(",")[0]?.trim() ?? "";
}

function deriveExpectedOrigin(request: RequestLike): string | null {
  let fallbackURL: URL;
  try {
    fallbackURL = new URL(request.url);
  } catch {
    return null;
  }

  const proto = firstForwardedValue(request.headers.get("x-forwarded-proto")) || fallbackURL.protocol.replace(":", "");
  const host = firstForwardedValue(request.headers.get("x-forwarded-host")) || request.headers.get("host") || fallbackURL.host;
  if (!proto || !host) {
    return null;
  }
  const normalizedProto = proto.toLowerCase();
  if (normalizedProto !== "http" && normalizedProto !== "https") {
    return null;
  }

  return `${normalizedProto}://${host.toLowerCase()}`;
}

function readAllowedOrigins(): Set<string> {
  const out = new Set<string>();
  const raw = process.env.LABTETHER_CSRF_ALLOWED_ORIGINS?.trim() ?? "";
  for (const token of raw.split(",")) {
    const candidate = token.trim();
    if (!candidate) {
      continue;
    }
    try {
      const parsed = new URL(candidate);
      out.add(parsed.origin.toLowerCase());
    } catch {
      // Ignore malformed allowlist entries.
    }
  }
  return out;
}

export function isMutatingMethod(method: string): boolean {
  return mutatingMethods.has(method.toUpperCase());
}

export function isMutationRequestOriginAllowed(request: RequestLike): boolean {
  if (!isMutatingMethod(request.method)) {
    return true;
  }

  const hasSession = hasLabtetherSessionCookie(request.headers.get("cookie"));
  if (!hasSession && hasServiceModeAuthHeader(request.headers)) {
    const origin = request.headers.get("origin");
    if (!origin) {
      return true; // Non-browser client (CLI, agent) — no Origin header
    }
    // Browser with service token — fall through to origin validation below
  }

  const originHeader = request.headers.get("origin")?.trim() ?? "";
  if (originHeader === "") {
    const fetchSite = (request.headers.get("sec-fetch-site") ?? "").trim().toLowerCase();
    return fetchSite === "" || fetchSite === "same-origin" || fetchSite === "same-site" || fetchSite === "none";
  }

  let parsedOrigin: URL;
  try {
    parsedOrigin = new URL(originHeader);
  } catch {
    return false;
  }
  if (parsedOrigin.protocol !== "http:" && parsedOrigin.protocol !== "https:") {
    return false;
  }

  const normalizedOrigin = parsedOrigin.origin.toLowerCase();
  const expectedOrigin = deriveExpectedOrigin(request);
  if (expectedOrigin && normalizedOrigin === expectedOrigin) {
    return true;
  }
  return readAllowedOrigins().has(normalizedOrigin);
}

export function isProxyRequestAuthorized(pathname: string, headers: Headers): boolean {
  if (hasServiceModeAuthHeader(headers) && !isTrustedServiceProxyPath(pathname)) {
    return false;
  }
  if (hasLabtetherSessionCookie(headers.get("cookie"))) {
    return true;
  }
  return isTrustedServiceProxyPath(pathname) && hasServiceModeAuthHeader(headers);
}
