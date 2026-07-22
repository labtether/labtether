import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
} from "../../../../lib/backend";
import {
  readBoundedRequestBody,
  RequestBodyTooLargeError,
} from "../../../../lib/boundedBody";
import { markResponseNoStore } from "../../../../lib/noStoreResponse";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";

export const dynamic = "force-dynamic";
export const maxDesktopSessionRequestBodyBytes = 32 * 1024;

const MAX_TARGET_BYTES = 512;
const MAX_HOST_BYTES = 255;
const MAX_DISPLAY_BYTES = 256;
const MAX_USERNAME_BYTES = 256;
const MAX_PASSWORD_BYTES = 16 * 1024;
const UPSTREAM_TIMEOUT_MS = 15_000;
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f-\u009f]/u;

type DesktopProtocol = "vnc" | "rdp" | "spice" | "webrtc";

type CreateDesktopSessionRequest = {
  target?: string;
  quality: "low" | "medium" | "high";
  display?: string;
  protocol: DesktopProtocol;
  record?: boolean;
  direct_target?: {
    host: string;
    port: number;
    username?: string;
    password?: string;
  };
};

type SessionPayload = {
  id?: string;
  target?: string;
  mode?: string;
  error?: string;
};

function byteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function validBoundedText(
  value: string,
  maxBytes: number,
  allowEmpty = false,
): boolean {
  return (
    (allowEmpty || value.length > 0) &&
    byteLength(value) <= maxBytes &&
    !CONTROL_CHARACTERS.test(value)
  );
}

export function parseDesktopRequest(
  raw: unknown,
): CreateDesktopSessionRequest | null {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    return null;
  }
  const obj = raw as Record<string, unknown>;

  if (obj.target !== undefined && typeof obj.target !== "string") return null;
  const target = typeof obj.target === "string" ? obj.target.trim() : "";
  if (target && !validBoundedText(target, MAX_TARGET_BYTES)) return null;

  const protocolRaw =
    obj.protocol === undefined
      ? "vnc"
      : typeof obj.protocol === "string"
        ? obj.protocol.trim().toLowerCase()
        : "";
  if (
    protocolRaw !== "vnc" &&
    protocolRaw !== "rdp" &&
    protocolRaw !== "spice" &&
    protocolRaw !== "webrtc"
  ) {
    return null;
  }

  const qualityRaw =
    obj.quality === undefined
      ? "medium"
      : typeof obj.quality === "string"
        ? obj.quality.trim().toLowerCase()
        : "";
  if (
    qualityRaw !== "low" &&
    qualityRaw !== "medium" &&
    qualityRaw !== "high"
  ) {
    return null;
  }

  if (obj.display !== undefined && typeof obj.display !== "string") return null;
  const display = typeof obj.display === "string" ? obj.display.trim() : "";
  if (display && !validBoundedText(display, MAX_DISPLAY_BYTES)) return null;

  if (obj.record !== undefined && typeof obj.record !== "boolean") return null;

  let directTarget: CreateDesktopSessionRequest["direct_target"];
  if (obj.direct_target !== undefined) {
    if (
      typeof obj.direct_target !== "object" ||
      obj.direct_target === null ||
      Array.isArray(obj.direct_target)
    ) {
      return null;
    }
    const direct = obj.direct_target as Record<string, unknown>;
    if (
      typeof direct.host !== "string" ||
      typeof direct.port !== "number" ||
      !Number.isInteger(direct.port) ||
      direct.port < 1 ||
      direct.port > 65_535
    ) {
      return null;
    }
    const host = direct.host.trim();
    if (!validBoundedText(host, MAX_HOST_BYTES)) return null;
    if (
      direct.username !== undefined &&
      typeof direct.username !== "string"
    ) {
      return null;
    }
    if (
      direct.password !== undefined &&
      typeof direct.password !== "string"
    ) {
      return null;
    }
    const username =
      typeof direct.username === "string" ? direct.username : undefined;
    const password =
      typeof direct.password === "string" ? direct.password : undefined;
    if (
      username !== undefined &&
      !validBoundedText(username, MAX_USERNAME_BYTES, true)
    ) {
      return null;
    }
    if (password !== undefined && byteLength(password) > MAX_PASSWORD_BYTES) {
      return null;
    }
    directTarget = {
      host,
      port: direct.port,
      username,
      password,
    };
  }

  if (!target && !directTarget) return null;
  if (directTarget && protocolRaw === "webrtc") return null;

  return {
    target: target || undefined,
    quality: qualityRaw,
    display: display || undefined,
    protocol: protocolRaw,
    record: typeof obj.record === "boolean" ? obj.record : undefined,
    direct_target: directTarget,
  };
}

function desktopJSON(payload: unknown, status = 200): Response {
  return markResponseNoStore(NextResponse.json(payload, { status }));
}

async function readDesktopRequestJSON(request: Request): Promise<unknown> {
  const raw = await readBoundedRequestBody(
    request,
    maxDesktopSessionRequestBodyBytes,
  );
  const text = new TextDecoder("utf-8", { fatal: true }).decode(raw);
  return JSON.parse(text);
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return desktopJSON({ error: "forbidden origin" }, 403);
  }

  let raw: unknown;
  try {
    raw = await readDesktopRequestJSON(request);
  } catch (error) {
    if (error instanceof RequestBodyTooLargeError) {
      return desktopJSON({ error: "request body too large" }, 413);
    }
    return desktopJSON({ error: "invalid JSON payload" }, 400);
  }

  const body = parseDesktopRequest(raw);
  if (!body) {
    return desktopJSON({ error: "invalid remote view session request" }, 400);
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/desktop/sessions`, {
      method: "POST",
      cache: "no-store",
      signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(body),
    });

    const payload = ((await safeJSON(response)) ?? {}) as SessionPayload;
    if (!response.ok || !payload.id) {
      return desktopJSON(
        upstreamErrorPayload(
          response.status,
          payload,
          "failed to create remote view session",
        ),
        response.status,
      );
    }

    return desktopJSON(
      {
        sessionId: payload.id,
        session: payload,
      },
      201,
    );
  } catch {
    return desktopJSON(
      { error: "remote view session endpoint unavailable" },
      502,
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
