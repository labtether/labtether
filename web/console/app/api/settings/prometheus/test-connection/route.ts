import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { sanitizeTestRouteError } from "../../testErrorSanitizer";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import { readBoundedRequestBody, RequestBodyTooLargeError } from "../../../../../lib/boundedBody";

export const dynamic = "force-dynamic";

const maxPrometheusTestBodyBytes = 24 * 1024;

function jsonNoStore(payload: unknown, status = 200) {
  return NextResponse.json(payload, {
    status,
    headers: { "Cache-Control": "no-store" },
  });
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return jsonNoStore({ error: "forbidden origin" }, 403);
  }

  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let parsed: Record<string, unknown>;
  try {
    const raw = await readBoundedRequestBody(request, maxPrometheusTestBodyBytes);
    const decoded = new TextDecoder("utf-8", { fatal: true }).decode(raw);
    const candidate: unknown = JSON.parse(decoded);
    if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) {
      throw new SyntaxError("request must be an object");
    }
    parsed = candidate as Record<string, unknown>;
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return jsonNoStore({ error: "request body too large" }, 413);
    }
    return jsonNoStore({ error: "invalid prometheus test connection payload" }, 400);
  }

  if (
    (parsed.url !== undefined && typeof parsed.url !== "string") ||
    (parsed.username !== undefined && typeof parsed.username !== "string") ||
    (parsed.password !== undefined && typeof parsed.password !== "string") ||
    (parsed.use_stored_password !== undefined && typeof parsed.use_stored_password !== "boolean")
  ) {
    return jsonNoStore({ error: "invalid prometheus test connection payload" }, 400);
  }

  const url = typeof parsed.url === "string" ? parsed.url : "";
  const username = typeof parsed.username === "string" ? parsed.username : "";
  const password = typeof parsed.password === "string" ? parsed.password : "";
  const useStoredPassword = parsed.use_stored_password === true;

  const cleanBody = JSON.stringify({ url, username, password, use_stored_password: useStoredPassword });
  const secretCandidates = [password, url];

  try {
    const response = await fetch(`${base.api}/settings/prometheus/test-connection`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
      },
      body: cleanBody,
      signal: AbortSignal.timeout(20_000),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return jsonNoStore(
        sanitizeTestRouteError(payload, "failed to test prometheus remote_write connection", secretCandidates),
        response.status,
      );
    }
    if (!payload || typeof payload !== "object" || typeof (payload as Record<string, unknown>).success !== "boolean") {
      return jsonNoStore({ error: "invalid prometheus remote_write response" }, 502);
    }
    const result = payload as Record<string, unknown>;
    if (result.success === true) {
      return jsonNoStore({ success: true });
    }
    const sanitized = sanitizeTestRouteError(result, "prometheus remote_write connection failed", secretCandidates);
    return jsonNoStore({ success: false, error: sanitized.error });
  } catch (error) {
    return jsonNoStore(
      sanitizeTestRouteError(
        error instanceof Error ? error.message : "",
        "failed to test prometheus remote_write connection",
        secretCandidates,
      ),
      502,
    );
  }
}
