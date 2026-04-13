import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
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
  username?: string;
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

    const collector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "pbs");
    if (!collector) {
      return NextResponse.json({
        configured: false,
        settings: {
          base_url: "",
          token_id: "",
          display_name: "",
          skip_verify: false,
          ca_pem: "",
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
        token_id: stringValue(config["token_id"]) || credential?.username || "",
        display_name: stringValue(config["display_name"]),
        skip_verify: boolValue(config["skip_verify"], false),
        ca_pem: stringValue(config["ca_pem"]),
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60),
      },
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load pbs settings" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  if (!isMutationRequestOriginAllowed(request)) {
    return NextResponse.json({ error: "forbidden origin" }, { status: 403 });
  }

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
  const tokenID = stringValue(body.token_id);
  const tokenSecret = stringValue(body.token_secret);
  const displayName = stringValue(body.display_name);
  const skipVerify = boolValue(body.skip_verify, false);
  const caPEM = stringValue(body.ca_pem);
  const intervalSeconds = clampInterval(numberValue(body.interval_seconds, 60));

  if (!baseURL || !tokenID) {
    return NextResponse.json({ error: "base_url and token_id are required" }, { status: 400 });
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
    const existingCollector = findCollectorByBaseURL(collectorsPayload?.collectors ?? [], "pbs", normalizedBaseURL);

    let credentialID = stringValue(existingCollector?.config?.["credential_id"]);
    let pendingRotateSecret: string | null = null;
    if (!credentialID) {
      if (!tokenSecret) {
        return NextResponse.json({ error: "token_secret is required for initial setup" }, { status: 400 });
      }
      const credName = displayName || hostLabelFromURL(normalizedBaseURL, "pbs");
      const credRes = await fetch(`${base.api}/credentials/profiles`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        body: JSON.stringify({
          name: `PBS API Token (${credName})`,
          kind: "pbs_api_token",
          username: tokenID,
          secret: tokenSecret,
          metadata: {
            base_url: normalizedBaseURL,
            display_name: displayName,
          },
        }),
      });
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile; error?: string } | null;
      if (!credRes.ok || !credPayload?.profile?.id) {
        return NextResponse.json(
          credPayload ?? { error: "failed to create pbs credential profile" },
          { status: credRes.status || 500 },
        );
      }
      credentialID = credPayload.profile.id;
    } else if (tokenSecret) {
      pendingRotateSecret = tokenSecret;
    }

    let assetID: string | undefined;
    if (!existingCollector?.id) {
      const label = displayName || hostLabelFromURL(normalizedBaseURL, "pbs");
      assetID = `pbs-server-${slugify(label, "pbs")}`;
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
          type: "storage-controller",
          name: label,
          source: "pbs",
          status: "online",
          metadata: {
            connector_type: "pbs",
            base_url: normalizedBaseURL,
          },
        }),
      });
      if (!heartbeatRes.ok) {
        const payload = await safeJSON(heartbeatRes);
        return NextResponse.json(payload ?? { error: "failed to create pbs asset" }, { status: heartbeatRes.status });
      }
    }

    const pbsConfig = {
      base_url: normalizedBaseURL,
      token_id: tokenID,
      display_name: displayName,
      credential_id: credentialID,
      skip_verify: skipVerify,
      ca_pem: caPEM,
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
          config: pbsConfig,
        }),
      });
      const patchPayload = await safeJSON(patchRes);
      if (!patchRes.ok) {
        return NextResponse.json(patchPayload ?? { error: "failed to update pbs collector" }, { status: patchRes.status });
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
          collector_type: "pbs",
          enabled: true,
          interval_seconds: intervalSeconds,
          config: pbsConfig,
        }),
      });
      const createPayload = await safeJSON(createRes);
      if (!createRes.ok) {
        return NextResponse.json(createPayload ?? { error: "failed to create pbs collector" }, { status: createRes.status });
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
          reason: "updated from settings pbs section",
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
      { error: error instanceof Error ? error.message : "failed to save pbs settings" },
      { status: 502 },
    );
  }
}

