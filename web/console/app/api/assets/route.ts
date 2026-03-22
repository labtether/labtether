import { NextResponse } from "next/server";
import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const incoming = new URL(request.url);

    const assetsURL = new URL(`${base.api}/assets`);
    // Forward supported query params
    const resolveComposites = incoming.searchParams.get("resolve_composites");
    if (resolveComposites) {
      assetsURL.searchParams.set("resolve_composites", resolveComposites);
    }
    const groupID = incoming.searchParams.get("group_id");
    if (groupID) {
      assetsURL.searchParams.set("group_id", groupID);
    }
    const tag = incoming.searchParams.get("tag");
    if (tag) {
      assetsURL.searchParams.set("tag", tag);
    }

    const res = await fetch(assetsURL.toString(), {
      cache: "no-store",
      headers: { ...backendAuthHeadersWithCookie(request) },
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "failed to list assets" }, { status: 500 });
  }
}
