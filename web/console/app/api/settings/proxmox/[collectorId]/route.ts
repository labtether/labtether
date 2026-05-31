import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../../lib/proxyAuth";
import { boolValue, clampInterval, numberValue, safeJSON, stringValue } from "../../shared";

export const dynamic = "force-dynamic";

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
  username?: string;
};

export async function GET(
  request: Request,
  { params }: { params: Promise<{ collectorId: string }> },
) {
  const { collectorId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  try {
    const collectorRes = await fetch(
      `${base.api}/hub-collectors/${encodeURIComponent(collectorId)}`,
      { cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(10_000) },
    );
    const collectorPayload = (await safeJSON(collectorRes)) as { collector?: HubCollector; error?: string } | null;
    if (!collectorRes.ok) {
      return NextResponse.json(
        collectorPayload ?? { error: "failed to load hub collector" },
        { status: collectorRes.status },
      );
    }

    const collector = collectorPayload?.collector;
    if (!collector) {
      return NextResponse.json({ error: "collector not found" }, { status: 404 });
    }

    const config = collector.config ?? {};
    const credentialID = stringValue(config["credential_id"]);
    const authMethod = stringValue(config["auth_method"]) || "api_token";
    let credential: CredentialProfile | null = null;
    if (credentialID) {
      const credRes = await fetch(
        `${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}`,
        { cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(10_000) },
      );
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile } | null;
      if (credRes.ok && credPayload?.profile) {
        credential = credPayload.profile;
      }
    }

    return NextResponse.json({
      configured: true,
      collector_id: collector.id,
      credential_id: credentialID,
      credential_name: credential?.name ?? "",
      settings: {
        base_url: stringValue(config["base_url"]),
        auth_method: authMethod,
        token_id: authMethod === "password" ? "" : (stringValue(config["token_id"]) || credential?.username || ""),
        username: authMethod === "password" ? (stringValue(config["username"]) || credential?.username || "") : "",
        skip_verify: boolValue(config["skip_verify"], false),
        ca_pem: stringValue(config["ca_pem"]),
        cluster_name: stringValue(config["cluster_name"]),
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60),
      },
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load collector settings" },
      { status: 502 },
    );
  }
}

export async function POST(
  request: Request,
  { params }: { params: Promise<{ collectorId: string }> },
) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

  const { collectorId } = await params;
  const base = await resolvedBackendBaseURLs();
  const authHeaders = backendAuthHeadersWithCookie(request);

  let body: Record<string, unknown> = {};
  try {
    body = (await request.json()) as Record<string, unknown>;
  } catch {
    body = {};
  }

  const baseURL = stringValue(body.base_url);
  const authMethod = stringValue(body.auth_method) || "api_token";
  const tokenID = stringValue(body.token_id);
  const tokenSecret = stringValue(body.token_secret);
  const username = stringValue(body.username);
  const skipVerify = boolValue(body.skip_verify, false);
  const caPEM = stringValue(body.ca_pem);
  const clusterName = stringValue(body.cluster_name);
  const intervalSeconds = clampInterval(numberValue(body.interval_seconds, 60));

  // Validate per auth method
  if (authMethod === "password") {
    if (!baseURL || !username) {
      return NextResponse.json({ error: "base_url and username are required" }, { status: 400 });
    }
  } else {
    if (!baseURL || !tokenID) {
      return NextResponse.json({ error: "base_url and token_id are required" }, { status: 400 });
    }
  }

  try {
    // Fetch existing collector to get credential_id
    const collectorRes = await fetch(
      `${base.api}/hub-collectors/${encodeURIComponent(collectorId)}`,
      { cache: "no-store", headers: authHeaders, signal: AbortSignal.timeout(10_000) },
    );
    const collectorPayload = (await safeJSON(collectorRes)) as { collector?: HubCollector; error?: string } | null;
    if (!collectorRes.ok) {
      return NextResponse.json(
        collectorPayload ?? { error: "failed to load hub collector" },
        { status: collectorRes.status },
      );
    }

    const existingCollector = collectorPayload?.collector;
    if (!existingCollector) {
      return NextResponse.json({ error: "collector not found" }, { status: 404 });
    }

    const credentialID = stringValue(existingCollector.config?.["credential_id"]);
    let pendingRotateSecret: string | null = null;

    if (tokenSecret && credentialID) {
      pendingRotateSecret = tokenSecret;
    }

    const proxmoxConfig: Record<string, unknown> = {
      base_url: baseURL,
      auth_method: authMethod,
      credential_id: credentialID,
      skip_verify: skipVerify,
      ca_pem: caPEM,
      cluster_name: clusterName,
      interval_seconds: intervalSeconds,
    };
    if (authMethod === "password") {
      proxmoxConfig.username = username;
    } else {
      proxmoxConfig.token_id = tokenID;
    }

    const patchRes = await fetch(
      `${base.api}/hub-collectors/${encodeURIComponent(collectorId)}`,
      {
        method: "PATCH",
        cache: "no-store",
        headers: { "Content-Type": "application/json", ...authHeaders },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          enabled: true,
          interval_seconds: intervalSeconds,
          config: proxmoxConfig,
        }),
      },
    );
    const patchPayload = await safeJSON(patchRes);
    if (!patchRes.ok) {
      return NextResponse.json(
        patchPayload ?? { error: "failed to update collector" },
        { status: patchRes.status },
      );
    }

    if (pendingRotateSecret && credentialID) {
      const rotateRes = await fetch(
        `${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}/rotate`,
        {
          method: "POST",
          cache: "no-store",
          headers: { "Content-Type": "application/json", ...authHeaders },
          signal: AbortSignal.timeout(10_000),
          body: JSON.stringify({
            secret: pendingRotateSecret,
            reason: "updated from device settings tab",
          }),
        },
      );
      if (!rotateRes.ok) {
        return NextResponse.json({
          configured: true,
          credential_id: credentialID,
          result: patchPayload,
          warning: "collector updated but credential rotation failed",
        });
      }
    }

    return NextResponse.json({
      configured: true,
      credential_id: credentialID,
      result: patchPayload,
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to save collector settings" },
      { status: 502 },
    );
  }
}
