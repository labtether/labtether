import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import createMiddleware from "next-intl/middleware";
import { isDemoModeEnabled } from "./app/lib/demoMode";
import { demoEntryRedirect } from "./lib/demoEntry";
import { hasLabtetherSessionCookie, isMutationRequestOriginAllowed, isMutatingMethod, isProxyRequestAuthorized } from "./lib/proxyAuth";

const locales = ["en", "de", "fr", "es", "zh"] as const;

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

  if (pathname === "/api" || pathname.startsWith("/api/")) {
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

  const demoEntry = demoEntryRedirect({
    demoMode: isDemoModeEnabled(),
    method: request.method,
    pathname,
    search: new URL(request.url).search,
    cookieHeader: request.headers.get("cookie"),
  });
  if (demoEntry) {
    // Resolved against the request, like the locale redirects, so the visitor
    // stays on the address they came in on.
    return NextResponse.redirect(new URL(demoEntry, request.url), 307);
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
    // File transfer route handlers enforce auth and CSRF locally so request
    // bodies can stream directly to the hub without proxy.ts buffering or
    // truncating large uploads.
    "/api/((?!files(?:/|$)).*)",
    "/api",
    // WebSocket upgrades that next.config.ts rewrites to the runtime proxy must
    // skip locale routing: a redirect to /en/... leaves the upgrade hanging, so a
    // Portainer container Shell never connected.
    "/((?!_next|api/files(?:/|$)|mcp$|ws|desktop/sessions|terminal/sessions|portainer/assets/.+/containers/.+/exec$|.*\\..*).*)"
  ]
};
