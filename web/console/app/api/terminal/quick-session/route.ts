import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

type QuickSessionRequest = {
  host: string;
  port?: number;
  username: string;
  auth_method: "password" | "private_key";
  password?: string;
  private_key?: string;
  passphrase?: string;
  strict_host_key?: boolean;
};

type SessionPayload = {
  session?: {
    id: string;
    target: string;
    mode: string;
  };
  error?: string;
};

function parseQuickSessionRequest(raw: unknown): QuickSessionRequest | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.host !== "string" || !obj.host.trim()) return null;
  if (typeof obj.username !== "string" || !obj.username.trim()) return null;
  if (typeof obj.auth_method !== "string") return null;
  if (obj.auth_method !== "password" && obj.auth_method !== "private_key") return null;
  return {
    host: obj.host.trim(),
    port: typeof obj.port === "number" ? obj.port : undefined,
    username: obj.username.trim(),
    auth_method: obj.auth_method,
    password: typeof obj.password === "string" ? obj.password : undefined,
    private_key: typeof obj.private_key === "string" ? obj.private_key : undefined,
    passphrase: typeof obj.passphrase === "string" ? obj.passphrase : undefined,
    strict_host_key: typeof obj.strict_host_key === "boolean" ? obj.strict_host_key : undefined,
  };
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  let raw: unknown;
  try {
    raw = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const body = parseQuickSessionRequest(raw);
  if (!body) {
    return NextResponse.json(
      { error: "host, username, and auth_method (password | private_key) are required" },
      { status: 400 },
    );
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/terminal/quick-session`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(body),
    });

    const payload = ((await safeJSON(response)) ?? {}) as SessionPayload;
    if (!response.ok || !payload.session?.id) {
      return NextResponse.json(
        { error: payload.error || "failed to create quick session" },
        { status: response.status || 502 },
      );
    }

    return NextResponse.json({
      sessionId: payload.session.id,
      session: payload.session,
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "quick-session endpoint unavailable" },
      { status: 502 },
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
