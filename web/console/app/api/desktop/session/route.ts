import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

type CreateDesktopSessionRequest = {
  target: string;
  quality?: string;
  display?: string;
  protocol?: "vnc" | "rdp" | "spice" | "webrtc";
  record?: boolean;
};

type SessionPayload = {
  id?: string;
  target?: string;
  mode?: string;
  error?: string;
};

function parseDesktopRequest(raw: unknown): CreateDesktopSessionRequest | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.target !== "string" || !obj.target.trim()) return null;
  const protocol = typeof obj.protocol === "string" ? obj.protocol.trim().toLowerCase() : "";
  const normalizedProtocol =
    protocol === "rdp" || protocol === "spice" || protocol === "webrtc" ? protocol : "vnc";
  return {
    target: obj.target.trim(),
    quality: typeof obj.quality === "string" ? obj.quality.trim() : undefined,
    display: typeof obj.display === "string" ? obj.display.trim() : undefined,
    protocol: normalizedProtocol as "vnc" | "rdp" | "spice" | "webrtc",
    record: typeof obj.record === "boolean" ? obj.record : undefined,
  };
}

export async function POST(request: Request) {
  let raw: unknown;
  try {
    raw = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const body = parseDesktopRequest(raw);
  if (!body) {
    return NextResponse.json({ error: "target is required" }, { status: 400 });
  }

  const target = body.target;

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/desktop/sessions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request)
      },
      body: JSON.stringify({
        target,
        quality: body.quality || "medium",
        display: body.display,
        protocol: body.protocol || "vnc",
        record: body.record ?? false,
      })
    });

    const payload = ((await safeJSON(response)) ?? {}) as SessionPayload;
    if (!response.ok || !payload.id) {
      return NextResponse.json({ error: payload.error || "failed to create remote view session" }, { status: response.status || 502 });
    }

    return NextResponse.json({
      sessionId: payload.id,
      session: payload
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "remote view session endpoint unavailable" },
      { status: 502 }
    );
  }
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
