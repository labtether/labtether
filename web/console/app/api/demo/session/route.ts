import { resolvedBackendBaseURLs } from "../../../../lib/backend";
import { safeLocalRedirectPath } from "../../../../lib/safeRedirect";

export const dynamic = "force-dynamic";

export async function HEAD() {
  const base = await resolvedBackendBaseURLs();
  try {
    const response = await fetch(`${base.api}/api/demo/session`, {
      method: "HEAD",
      redirect: "manual",
      cache: "no-store",
    });
    if (response.status === 204) {
      return new Response(null, { status: 200 });
    }
    if (response.status === 404) {
      return new Response(null, { status: 204 });
    }
    return new Response(null, { status: 502 });
  } catch {
    return new Response(null, { status: 502 });
  }
}

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const url = new URL(request.url);
  const redirect = safeLocalRedirectPath(url.searchParams.get("redirect"));

  try {
    // Proxy to hub's demo session endpoint (which creates the session + sets cookie).
    const hubURL = `${base.api}/api/demo/session?redirect=${encodeURIComponent(redirect)}`;
    const response = await fetch(hubURL, {
      method: "GET",
      redirect: "manual",
      cache: "no-store",
    });

    if (response.status < 300 || response.status >= 400) {
      return new Response(null, { status: response.status });
    }

    // The hub responds with a redirect + Set-Cookie. Forward both.
    const cookies = response.headers.getSetCookie();
    const location = safeLocalRedirectPath(response.headers.get("location"), redirect);

    if (cookies.length === 0) {
      return new Response(null, { status: 502 });
    }

    const res = new Response(null, {
      status: 307,
      headers: { Location: location },
    });

    for (const cookie of cookies) {
      res.headers.append("set-cookie", cookie);
    }

    return res;
  } catch {
    return new Response("Demo session unavailable", { status: 502 });
  }
}
