import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";

type ExecuteRequest = {
  target: string;
  command: string;
  actorId?: string;
  sessionId?: string;
};

type SessionPayload = {
  session?: {
    id: string;
    target: string;
    mode: string;
  };
  error?: string;
};

type CommandPayload = {
  job_id?: string;
  command?: {
    id: string;
    session_id: string;
    status: string;
    body: string;
  };
  status?: string;
  error?: string;
};

export async function POST(request: Request) {
  let body: ExecuteRequest;
  try {
    body = (await request.json()) as ExecuteRequest;
  } catch {
    return NextResponse.json({ error: "invalid JSON payload" }, { status: 400 });
  }

  const target = body.target?.trim();
  const command = body.command?.trim();
  const actorId = body.actorId?.trim() || "owner";

  if (!target) {
    return NextResponse.json({ error: "target is required" }, { status: 400 });
  }
  if (!command) {
    return NextResponse.json({ error: "command is required" }, { status: 400 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  let sessionID = body.sessionId?.trim();

  if (!sessionID) {
    const sessionResponse = await fetch(`${base.api}/terminal/sessions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders
      },
      body: JSON.stringify({
        actor_id: actorId,
        target,
        mode: "interactive"
      })
    });

    const sessionPayload = (await safeJSON<SessionPayload>(sessionResponse)) ?? {};

    if (!sessionResponse.ok || !sessionPayload.session?.id) {
      return NextResponse.json(
        { error: sessionPayload.error || "failed to create terminal session" },
        { status: sessionResponse.status || 502 }
      );
    }

    sessionID = sessionPayload.session.id;
  }

  const commandResponse = await fetch(`${base.api}/terminal/sessions/${encodeURIComponent(sessionID)}/commands`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...authHeaders
    },
    body: JSON.stringify({
      actor_id: actorId,
      command
    })
  });

  const commandPayload = (await safeJSON<CommandPayload>(commandResponse)) ?? {};

  if (!commandResponse.ok) {
    return NextResponse.json(
      { error: commandPayload.error || "failed to enqueue command" },
      { status: commandResponse.status || 502 }
    );
  }

  return NextResponse.json({
    sessionId: sessionID,
    jobId: commandPayload.job_id,
    command: commandPayload.command,
    status: commandPayload.status || "queued"
  });
}

async function safeJSON<T>(response: Response): Promise<T | null> {
  try {
    return (await response.json()) as T;
  } catch {
    return null;
  }
}
