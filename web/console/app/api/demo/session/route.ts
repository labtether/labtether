import { resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const url = new URL(request.url);
  const redirect = url.searchParams.get("redirect") || "/";

  try {
    // Proxy to hub's demo session endpoint (which creates the session + sets cookie).
    const hubURL = `${base.api}/api/demo/session?redirect=${encodeURIComponent(redirect)}`;
    const response = await fetch(hubURL, {
      method: "GET",
      redirect: "manual",
      cache: "no-store",
    });

    // The hub responds with a redirect + Set-Cookie. Forward both.
    const cookies = response.headers.getSetCookie();
    const location = response.headers.get("location") || redirect;

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
