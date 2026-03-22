import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  isRoutingOverrideKey,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
  validateRoutingOverrideURL,
  type BackendBaseURLs,
} from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const response = await fetch(`${base.api}/settings/runtime`, {
      cache: "no-store",
      headers: authHeaders
    });

    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to load runtime settings"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {});
  } catch {
    return NextResponse.json({ error: "runtime settings endpoint unavailable" }, { status: 502 });
  }
}

export async function PATCH(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);
  const payload = await safeRequestJSON(request);
  if (payload === null) {
    return NextResponse.json({ error: "invalid request body" }, { status: 400 });
  }
  const preparedBody = sanitizeRuntimePatchBody(payload, base);
  if ("error" in preparedBody) {
    return NextResponse.json({ error: preparedBody.error }, { status: 400 });
  }

  try {
    const response = await fetch(`${base.api}/settings/runtime`, {
      method: "PATCH",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders
      },
      body: JSON.stringify(preparedBody.body)
    });

    const responsePayload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, responsePayload, "failed to update runtime settings"),
        { status: response.status },
      );
    }
    return NextResponse.json(responsePayload ?? {});
  } catch {
    return NextResponse.json({ error: "runtime settings endpoint unavailable" }, { status: 502 });
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

async function safeRequestJSON(request: Request): Promise<Record<string, unknown> | null> {
  try {
    const payload = await request.json();
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      return null;
    }
    return payload as Record<string, unknown>;
  } catch {
    return null;
  }
}

function sanitizeRuntimePatchBody(
  payload: Record<string, unknown>,
  base: BackendBaseURLs,
): { body: Record<string, unknown> } | { error: string } {
  const body: Record<string, unknown> = { ...payload };
  if (!("values" in body) || body.values === undefined) {
    return { body };
  }
  if (!body.values || typeof body.values !== "object" || Array.isArray(body.values)) {
    return { error: "values must be an object" };
  }

  const values = body.values as Record<string, unknown>;
  const sanitizedValues: Record<string, unknown> = {};
  for (const [key, rawValue] of Object.entries(values)) {
    if (!isRoutingOverrideKey(key)) {
      sanitizedValues[key] = rawValue;
      continue;
    }
    if (typeof rawValue !== "string") {
      return { error: `${key} must be a string URL` };
    }
    const validation = validateRoutingOverrideURL(key, rawValue, base);
    if (!validation.valid) {
      return { error: `${key} ${validation.reason}` };
    }
    sanitizedValues[key] = validation.normalized;
  }

  body.values = sanitizedValues;
  return { body };
}
