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
      signal: AbortSignal.timeout(10_000),
    });
    const collectorsPayload = (await safeJSON(collectorsRes)) as { collectors?: HubCollector[]; error?: string } | null;
    if (!collectorsRes.ok) {
      return NextResponse.json(
        collectorsPayload ?? { error: "failed to load hub collectors" },
        { status: collectorsRes.status }
      );
    }

    const collector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "portainer");
    if (!collector) {
      return NextResponse.json({
        configured: false,
        settings: {
          base_url: "",
          auth_method: "api_key",
          token_id: "",
          cluster_name: "",
          skip_verify: false,
          interval_seconds: 60
        }
      });
    }

    const config = collector.config ?? {};
    const credentialID = stringValue(config["credential_id"]);
    const authMethod = stringValue(config["auth_method"]) || "api_key";
    let credential: CredentialProfile | null = null;
    if (credentialID) {
      const credRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}`, {
        cache: "no-store",
        headers: authHeaders,
        signal: AbortSignal.timeout(10_000),
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
        auth_method: authMethod,
        token_id: authMethod === "password" ? "" : (stringValue(config["token_id"]) || credential?.username || ""),
        skip_verify: boolValue(config["skip_verify"], false),
        cluster_name: stringValue(config["cluster_name"]),
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60),
      }
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load portainer settings" },
      { status: 502 }
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
  const authMethod = stringValue(body.auth_method) || "api_key";
  const tokenID = stringValue(body.token_id);
  const tokenSecret = stringValue(body.token_secret);
  const clusterName = stringValue(body.cluster_name);
  const skipVerify = boolValue(body.skip_verify, false);
  const intervalSeconds = clampInterval(numberValue(body.interval_seconds, 60));

  if (authMethod !== "api_key") {
    return NextResponse.json({ error: "unsupported auth_method; only api_key is supported" }, { status: 400 });
  }
  if (!baseURL) {
    return NextResponse.json({ error: "base_url is required" }, { status: 400 });
  }

  try {
    const collectorsRes = await fetch(`${base.api}/hub-collectors?enabled=false`, {
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(10_000),
    });
    const collectorsPayload = (await safeJSON(collectorsRes)) as { collectors?: HubCollector[]; error?: string } | null;
    if (!collectorsRes.ok) {
      return NextResponse.json(
        collectorsPayload ?? { error: "failed to load hub collectors" },
        { status: collectorsRes.status }
      );
    }
    const existingCollector = findCollectorByBaseURL(
      collectorsPayload?.collectors ?? [],
      "portainer",
      normalizedBaseURL,
    );

    let credentialID = stringValue(existingCollector?.config?.["credential_id"]);
    let pendingRotateSecret: string | null = null;
    if (!credentialID) {
      if (!tokenSecret) {
        return NextResponse.json({ error: "token_secret is required for initial setup" }, { status: 400 });
      }

      const credRes = await fetch(`${base.api}/credentials/profiles`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          name: `Portainer API Key (${clusterName || hostLabelFromURL(normalizedBaseURL, "portainer")})`,
          kind: "portainer_api_key",
          username: tokenID,
          secret: tokenSecret,
          metadata: {
            base_url: normalizedBaseURL,
            cluster_name: clusterName
          }
        }),
      });
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile; error?: string } | null;
      if (!credRes.ok || !credPayload?.profile?.id) {
        return NextResponse.json(
          credPayload ?? { error: "failed to create portainer credential profile" },
          { status: credRes.status || 500 }
        );
      }
      credentialID = credPayload.profile.id;
    } else if (tokenSecret) {
      // Defer rotation until collector update succeeds to avoid stale collector+new secret mismatch.
      pendingRotateSecret = tokenSecret;
    }

    const portainerConfig = {
      base_url: normalizedBaseURL,
      auth_method: authMethod,
      token_id: tokenID,
      credential_id: credentialID,
      skip_verify: skipVerify,
      cluster_name: clusterName,
      interval_seconds: intervalSeconds
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
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          enabled: true,
          interval_seconds: intervalSeconds,
          config: portainerConfig,
        }),
      });
      const patchPayload = await safeJSON(patchRes);
      if (!patchRes.ok) {
        return NextResponse.json(
          patchPayload ?? { error: "failed to update portainer collector" },
          { status: patchRes.status }
        );
      }
      collectorResponsePayload = patchPayload;

      if (pendingRotateSecret) {
        const rotateRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}/rotate`, {
          method: "POST",
          cache: "no-store",
          headers: {
            "Content-Type": "application/json",
            ...authHeaders,
          },
          signal: AbortSignal.timeout(10_000),
          body: JSON.stringify({
            secret: pendingRotateSecret,
            reason: "updated from portainer settings"
          }),
        });
        if (!rotateRes.ok) {
          return NextResponse.json({
            configured: true,
            credential_id: credentialID,
            result: collectorResponsePayload,
            warning: "collector updated but credential rotation failed — secret may be out of sync"
          });
        }
      }
    } else {
      const assetID = `portainer-cluster-${slugify(clusterName || hostLabelFromURL(normalizedBaseURL, "portainer"), "portainer")}`;
      const heartbeatRes = await fetch(`${base.api}/assets/heartbeat`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          asset_id: assetID,
          type: "connector-cluster",
          name: clusterName || hostLabelFromURL(normalizedBaseURL, "portainer"),
          source: "portainer",
          status: "online",
          metadata: {
            connector_type: "portainer",
            base_url: normalizedBaseURL
          }
        }),
      });
      if (!heartbeatRes.ok) {
        const payload = await safeJSON(heartbeatRes);
        return NextResponse.json(payload ?? { error: "failed to create portainer cluster asset" }, { status: heartbeatRes.status });
      }

      const createRes = await fetch(`${base.api}/hub-collectors`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders,
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          asset_id: assetID,
          collector_type: "portainer",
          enabled: true,
          interval_seconds: intervalSeconds,
          config: portainerConfig
        }),
      });
      const createPayload = await safeJSON(createRes);
      if (!createRes.ok) {
        return NextResponse.json(createPayload ?? { error: "failed to create portainer collector" }, { status: createRes.status });
      }
      collectorResponsePayload = createPayload;
    }

    return NextResponse.json({
      configured: true,
      credential_id: credentialID,
      result: collectorResponsePayload
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to save portainer settings" },
      { status: 502 }
    );
  }
}
