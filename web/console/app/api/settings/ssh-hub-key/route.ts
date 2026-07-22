import { NextResponse } from "next/server";

import {
  backendAuthHeadersWithCookie,
  resolvedBackendBaseURLs,
  upstreamErrorPayload,
} from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import { safeJSON } from "../shared";

export const dynamic = "force-dynamic";

const ROTATION_CONFIRMATION = "ROTATE";
const MAX_ROTATION_BODY_BYTES = 4096;
const MAX_ROTATION_REASON_CHARACTERS = 256;

type RotationPayload = {
  key_type: "ed25519" | "rsa" | "";
  reason: string;
  confirm: string;
};

async function readRotationPayload(request: Request): Promise<
  | { payload: RotationPayload }
  | { error: string; status: number }
> {
  const declaredLength = Number(request.headers.get("content-length") ?? "0");
  if (Number.isFinite(declaredLength) && declaredLength > MAX_ROTATION_BODY_BYTES) {
    return { error: "rotation request is too large", status: 413 };
  }

  const rawBody = await request.text();
  if (new TextEncoder().encode(rawBody).byteLength > MAX_ROTATION_BODY_BYTES) {
    return { error: "rotation request is too large", status: 413 };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(rawBody);
  } catch {
    return { error: "invalid SSH hub key rotation request", status: 400 };
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return { error: "invalid SSH hub key rotation request", status: 400 };
  }

  const record = parsed as Record<string, unknown>;
  const keyType = typeof record.key_type === "string" ? record.key_type.trim().toLowerCase() : "";
  const rawReason = typeof record.reason === "string" ? record.reason : "";
  const reason = rawReason.trim();
  const confirm = typeof record.confirm === "string" ? record.confirm : "";

  if (keyType !== "" && keyType !== "ed25519" && keyType !== "rsa") {
    return { error: "key type must be ed25519 or rsa", status: 400 };
  }
  if (
    Array.from(rawReason).length > MAX_ROTATION_REASON_CHARACTERS
    || /[\u0000-\u001f\u007f-\u009f]/u.test(rawReason)
  ) {
    return { error: "rotation reason is invalid", status: 400 };
  }
  if (confirm !== ROTATION_CONFIRMATION) {
    return { error: `confirmation required: type ${ROTATION_CONFIRMATION}`, status: 400 };
  }

  return {
    payload: {
      key_type: keyType,
      reason,
      confirm,
    } as RotationPayload,
  };
}

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/settings/ssh-hub-key`, {
      cache: "no-store",
      headers: backendAuthHeadersWithCookie(request),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to load SSH hub key"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {}, {
      headers: { "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json({ error: "SSH hub key endpoint unavailable" }, { status: 502 });
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const parsed = await readRotationPayload(request);
  if ("error" in parsed) {
    return NextResponse.json({ error: parsed.error }, { status: parsed.status });
  }

  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/settings/ssh-hub-key/rotate`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body: JSON.stringify(parsed.payload),
    });
    const payload = await safeJSON(response);
    if (!response.ok) {
      return NextResponse.json(
        upstreamErrorPayload(response.status, payload, "failed to rotate SSH hub key"),
        { status: response.status },
      );
    }
    return NextResponse.json(payload ?? {}, {
      headers: { "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json({ error: "SSH hub key rotation endpoint unavailable" }, { status: 502 });
  }
}
