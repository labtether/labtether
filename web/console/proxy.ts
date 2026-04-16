import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import createMiddleware from "next-intl/middleware";
import { hasLabtetherSessionCookie, isMutationRequestOriginAllowed, isMutatingMethod, isProxyRequestAuthorized } from "./lib/proxyAuth";

const locales = ["en", "de", "fr", "es", "zh"] as const;

/** Runtime demo mode check — must be a function call so the bundler cannot
 *  statically evaluate it and tree-shake the guarded code path. */
function isDemoMode(): boolean {
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const env: Record<string, string | undefined> = (globalThis as any).process?.env ?? {};
    return env.NEXT_PUBLIC_DEMO_MODE === "true" || env.LABTETHER_DEMO_MODE === "true";
  } catch {
    return false;
  }
}

const publicAPIPaths = new Set<string>([
  "/api/auth/login",
  "/api/auth/login/2fa",
  "/api/auth/bootstrap",
  "/api/auth/bootstrap/status",
  "/api/auth/providers",
  "/api/auth/oidc/start",
  "/api/auth/oidc/callback",
  "/api/demo/session",
]);

const handleI18nRouting = createMiddleware({
  locales,
  defaultLocale: "en",
  localePrefix: "always",
});

export function proxy(request: NextRequest) {
  const { pathname } = new URL(request.url);

  // Demo mode: auto-provision a session for unauthenticated page visits.
  // NEXT_PUBLIC_ vars are inlined at build time. Turbopack aggressively
  // tree-shakes any branch guarded by a build-time-false condition, even
  // if OR'd with a runtime check. Use only a runtime check here.
  if (isDemoMode()) {
    const cookies = request.headers.get("cookie") ?? "";
    if (!hasLabtetherSessionCookie(cookies) && !pathname.startsWith("/api/")) {
      const redirect = encodeURIComponent(pathname);
      return Response.redirect(new URL(`/api/demo/session?redirect=${redirect}`, request.url), 307);
    }
  }

  if (pathname.startsWith("/api/") || pathname.startsWith("/api")) {
    if (request.method === "OPTIONS") {
      return NextResponse.next();
    }
    if (isMutatingMethod(request.method) && !isMutationRequestOriginAllowed(request)) {
      return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
    }
    if (publicAPIPaths.has(pathname)) {
      return NextResponse.next();
    }
    if (pathname.startsWith("/api/v1/")) {
      return NextResponse.next();
    }
    if (!isProxyRequestAuthorized(pathname, request.headers)) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }
    return NextResponse.next();
  }

  // In standalone/proxied runs, next-intl's default-locale "as-needed"
  // rewrite can be re-processed as a second middleware pass and bounce
  // between `/foo` and `/en/foo`. Let explicitly-prefixed routes pass
  // through so unprefixed routes can still rewrite internally without
  // triggering a self-redirect loop.
  const explicitLocale = locales.find(
    (locale) => pathname === `/${locale}` || pathname.startsWith(`/${locale}/`),
  );
  if (explicitLocale) {
    const headers = new Headers(request.headers);
    headers.set("X-NEXT-INTL-LOCALE", explicitLocale);
    const response = NextResponse.next({
      request: {
        headers,
      },
    });
    response.cookies.set("NEXT_LOCALE", explicitLocale, {
      path: "/",
      sameSite: "lax",
    });
    return response;
  }

  return handleI18nRouting(request);
}

export const config = {
  matcher: [
    "/api/:path*",
    "/((?!_next|ws|desktop/sessions|terminal/sessions|.*\\..*).*)"
  ]
};
