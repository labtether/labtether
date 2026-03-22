import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import {
  boolValue,
  clampInterval,
  findCollectorByBaseURL,
  hostLabelFromURL,
  normalizeBaseURL,
  numberValue,
  safeJSON,
  slugify,
  stringValue,
} from "../shared";

export const dynamic = "force-dynamic";
const FETCH_TIMEOUT_MS = 10_000;

type HubCollector = {
  id: string;
  asset_id: string;
  collector_type: string;
  enabled: boolean;
  interval_seconds: number;
  config?: Record<string, unknown>;
};

type CredentialProfile = {
  id: string;
  name: string;
  kind: string;
};

export async function GET(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const collectorsRes = await fetch(`${base.api}/hub-collectors?enabled=false`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
    });
    const collectorsPayload = (await safeJSON(collectorsRes)) as { collectors?: HubCollector[]; error?: string } | null;
    if (!collectorsRes.ok) {
      return NextResponse.json(
        collectorsPayload ?? { error: "failed to load hub collectors" },
        { status: collectorsRes.status },
      );
    }

    const collector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "homeassistant");
    if (!collector) {
      return NextResponse.json({
        configured: false,
        settings: {
          base_url: "",
          display_name: "",
          skip_verify: false,
          interval_seconds: 60,
        },
      });
    }

    const config = collector.config ?? {};
    const credentialID = stringValue(config["credential_id"]);
    let credential: CredentialProfile | null = null;
    if (credentialID) {
      const credRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}`, {
        cache: "no-store",
        headers: authHeaders,
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
      });
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile } | null;
      if (credRes.ok && credPayload?.profile) {
        credential = credPayload.profile;
      }
    }

    return NextResponse.json({
      configured: true,
      collector_id: collector.id,
      asset_id: collector.asset_id,
      credential_id: credentialID,
      credential_name: credential?.name ?? "",
      settings: {
        base_url: stringValue(config["base_url"]),
        display_name: stringValue(config["display_name"]),
        skip_verify: boolValue(config["skip_verify"], false),
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60),
      },
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load home assistant settings" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let body: Record<string, unknown> = {};
  try {
    body = (await request.json()) as Record<string, unknown>;
  } catch {
    body = {};
  }

  const baseURL = stringValue(body.base_url);
  const normalizedBaseURL = normalizeBaseURL(baseURL);
  const token = stringValue(body.token);
  const displayName = stringValue(body.display_name);
  const skipVerify = boolValue(body.skip_verify, false);
  const intervalSeconds = clampInterval(numberValue(body.interval_seconds, 60));

  if (!baseURL) {
    return NextResponse.json({ error: "base_url is required" }, { status: 400 });
  }

  try {
    const collectorsRes = await fetch(`${base.api}/hub-collectors?enabled=false`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
    });
    const collectorsPayload = (await safeJSON(collectorsRes)) as { collectors?: HubCollector[]; error?: string } | null;
    if (!collectorsRes.ok) {
      return NextResponse.json(
        collectorsPayload ?? { error: "failed to load hub collectors" },
        { status: collectorsRes.status },
      );
    }
    const existingCollector = findCollectorByBaseURL(collectorsPayload?.collectors ?? [], "homeassistant", normalizedBaseURL);

    let credentialID = stringValue(existingCollector?.config?.["credential_id"]);
    let pendingRotateSecret: string | null = null;
    if (!credentialID) {
      if (!token) {
        return NextResponse.json({ error: "token is required for initial setup" }, { status: 400 });
      }
      const credName = displayName || hostLabelFromURL(normalizedBaseURL, "homeassistant");
      const credRes = await fetch(`${base.api}/credentials/profiles`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        body: JSON.stringify({
          name: `Home Assistant Token (${credName})`,
          kind: "homeassistant_token",
          secret: token,
          metadata: {
            base_url: normalizedBaseURL,
            display_name: displayName,
          },
        }),
      });
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile; error?: string } | null;
      if (!credRes.ok || !credPayload?.profile?.id) {
        return NextResponse.json(
          credPayload ?? { error: "failed to create home assistant credential profile" },
          { status: credRes.status || 500 },
        );
      }
      credentialID = credPayload.profile.id;
    } else if (token) {
      pendingRotateSecret = token;
    }

    let assetID: string | undefined;
    if (!existingCollector?.id) {
      const label = displayName || hostLabelFromURL(normalizedBaseURL, "homeassistant");
      assetID = `homeassistant-hub-${slugify(label, "homeassistant")}`;
      const heartbeatRes = await fetch(`${base.api}/assets/heartbeat`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        body: JSON.stringify({
          asset_id: assetID,
          type: "connector-cluster",
          name: label,
          source: "homeassistant",
          status: "online",
          metadata: {
            connector_type: "homeassistant",
            base_url: normalizedBaseURL,
          },
        }),
      });
      if (!heartbeatRes.ok) {
        const payload = await safeJSON(heartbeatRes);
        return NextResponse.json(payload ?? { error: "failed to create home assistant asset" }, { status: heartbeatRes.status });
      }
    }

    const config = {
      base_url: normalizedBaseURL,
      display_name: displayName,
      credential_id: credentialID,
      skip_verify: skipVerify,
      interval_seconds: intervalSeconds,
    };

    let collectorResponsePayload: unknown;
    if (existingCollector?.id) {
      const patchRes = await fetch(`${base.api}/hub-collectors/${encodeURIComponent(existingCollector.id)}`, {
        method: "PATCH",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        body: JSON.stringify({
          enabled: true,
          interval_seconds: intervalSeconds,
          config,
        }),
      });
      const patchPayload = await safeJSON(patchRes);
      if (!patchRes.ok) {
        return NextResponse.json(patchPayload ?? { error: "failed to update home assistant collector" }, { status: patchRes.status });
      }
      collectorResponsePayload = patchPayload;
    } else {
      const createRes = await fetch(`${base.api}/hub-collectors`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        body: JSON.stringify({
          asset_id: assetID,
          collector_type: "homeassistant",
          enabled: true,
          interval_seconds: intervalSeconds,
          config,
        }),
      });
      const createPayload = await safeJSON(createRes);
      if (!createRes.ok) {
        return NextResponse.json(createPayload ?? { error: "failed to create home assistant collector" }, { status: createRes.status });
      }
      collectorResponsePayload = createPayload;
    }

    let warning = "";
    if (pendingRotateSecret) {
      const rotateRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}/rotate`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        body: JSON.stringify({
          secret: pendingRotateSecret,
          reason: "updated from settings home assistant section",
        }),
      });
      const rotatePayload = (await safeJSON(rotateRes)) as { error?: string } | null;
      if (!rotateRes.ok) {
        warning = rotatePayload?.error?.trim()
          ? `collector updated but credential rotation failed: ${rotatePayload.error.trim()}`
          : "collector updated but credential rotation failed; secret may be out of sync";
      }
    }

    return NextResponse.json({
      configured: true,
      credential_id: credentialID,
      result: collectorResponsePayload,
      warning,
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to save home assistant settings" },
      { status: 502 },
    );
  }
}

