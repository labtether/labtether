import { NextResponse } from "next/server";
import { resolvedBackendBaseURLs } from "../../../../lib/backend";

export const dynamic = "force-dynamic";

export async function GET() {
  const base = await resolvedBackendBaseURLs();
  try {
    const response = await fetch(`${base.api}/auth/providers`, {
      cache: "no-store",
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json({ local: { enabled: true }, oidc: { enabled: false } }, { status: 200 });
    }
    return NextResponse.json(payload ?? { local: { enabled: true }, oidc: { enabled: false } }, { status: 200 });
  } catch {
    return NextResponse.json({ local: { enabled: true }, oidc: { enabled: false } }, { status: 200 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
