import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

type CreateSessionRequest = {
  target: string;
  actorId?: string;
  mode?: "interactive" | "structured";
};

type SessionPayload = {
  session?: {
    id: string;
    target: string;
    mode: string;
  };
  error?: string;
};

function parseSessionRequest(raw: unknown): CreateSessionRequest | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.target !== "string" || !obj.target.trim()) return null;
  const mode = typeof obj.mode === "string" ? obj.mode.trim() : undefined;
  if (mode && mode !== "interactive" && mode !== "structured") return null;
  return {
    target: obj.target.trim(),
    actorId: typeof obj.actorId === "string" ? obj.actorId.trim() : undefined,
    mode: (mode as CreateSessionRequest["mode"]) || undefined,
  };
}

export async function POST(request: Request) {
  let raw: unknown;
  try {
    raw = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const body = parseSessionRequest(raw);
  if (!body) {
    return NextResponse.json({ error: "target is required and mode must be interactive or structured" }, { status: 400 });
  }

  const target = body.target;
  const actorId = body.actorId || "owner";
  const mode = body.mode || "interactive";

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/sessions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request)
      },
      body: JSON.stringify({
        actor_id: actorId,
        target,
        mode
      })
    });

    const payload = ((await safeJSON(response)) ?? {}) as SessionPayload;
    if (!response.ok || !payload.session?.id) {
      return NextResponse.json({ error: payload.error || "failed to create session" }, { status: response.status || 502 });
    }

    return NextResponse.json({
      sessionId: payload.session.id,
      session: payload.session
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "session endpoint unavailable" },
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
