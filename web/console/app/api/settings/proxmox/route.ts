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

    const collector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "proxmox");
    if (!collector) {
      return NextResponse.json({
        configured: false,
        settings: {
          base_url: "",
          auth_method: "api_token",
          token_id: "",
          username: "",
          skip_verify: false,
          ca_pem: "",
          cluster_name: "",
          interval_seconds: 60
        }
      });
    }

    const config = collector.config ?? {};
    const credentialID = stringValue(config["credential_id"]);
    const authMethod = stringValue(config["auth_method"]) || "api_token";
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
        username: authMethod === "password" ? (stringValue(config["username"]) || credential?.username || "") : "",
        skip_verify: boolValue(config["skip_verify"], false),
        ca_pem: stringValue(config["ca_pem"]),
        cluster_name: stringValue(config["cluster_name"]),
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60),
      }
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load proxmox settings" },
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
      "proxmox",
      normalizedBaseURL,
    );

    let credentialID = stringValue(existingCollector?.config?.["credential_id"]);
    let createdCredentialID = "";
    let createdAssetID = "";
    // Track whether a credential rotation is pending (deferred until after collector update)
    let pendingRotateSecret: string | null = null;

    // Determine credential kind and identity based on auth method
    const credKind = authMethod === "password" ? "proxmox_password" : "proxmox_api_token";
    const credUsername = authMethod === "password" ? username : tokenID;
    const credLabel = authMethod === "password"
      ? `Proxmox Password (${clusterName || hostLabelFromURL(normalizedBaseURL, "proxmox")})`
      : `Proxmox API Token (${clusterName || hostLabelFromURL(normalizedBaseURL, "proxmox")})`;

    const proxmoxConfig: Record<string, unknown> = {
      base_url: normalizedBaseURL,
      auth_method: authMethod,
      credential_id: credentialID,
      skip_verify: skipVerify,
      ca_pem: caPEM,
      cluster_name: clusterName,
      interval_seconds: intervalSeconds
    };
    if (authMethod === "password") {
      proxmoxConfig.username = username;
    } else {
      proxmoxConfig.token_id = tokenID;
    }

    let collectorResponsePayload: unknown;
    if (existingCollector?.id) {
      if (tokenSecret) {
        // Defer rotation until after the collector update succeeds
        pendingRotateSecret = tokenSecret;
      }
      const patchRes = await fetch(`${base.api}/hub-collectors/${encodeURIComponent(existingCollector.id)}`, {
        method: "PATCH",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          enabled: true,
          interval_seconds: intervalSeconds,
          config: proxmoxConfig
        })
      });
      const patchPayload = await safeJSON(patchRes);
      if (!patchRes.ok) {
        // Collector update failed — do NOT rotate credential; return error
        return NextResponse.json(patchPayload ?? { error: "failed to update proxmox collector" }, { status: patchRes.status });
      }
      collectorResponsePayload = patchPayload;

      // Collector update succeeded — now rotate the credential if requested
      if (pendingRotateSecret) {
        const rotateRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}/rotate`, {
          method: "POST",
          cache: "no-store",
          headers: {
            "Content-Type": "application/json",
            ...authHeaders
          },
          signal: AbortSignal.timeout(10_000),
          body: JSON.stringify({
            secret: pendingRotateSecret,
            reason: "updated from settings proxmox section"
          })
        });
        if (!rotateRes.ok) {
          // Collector config is correct; warn but don't fail
          return NextResponse.json({
            configured: true,
            credential_id: credentialID,
            result: collectorResponsePayload,
            warning: "collector updated but credential rotation failed — secret may be out of sync"
          });
        }
      }
    } else {
      if (!tokenSecret) {
        return NextResponse.json({ error: "secret/password is required for initial setup" }, { status: 400 });
      }

      const assetID = `proxmox-cluster-${slugify(clusterName || hostLabelFromURL(normalizedBaseURL, "proxmox"), "proxmox")}`;
      const existingAssetRes = await fetch(`${base.api}/assets/${encodeURIComponent(assetID)}`, {
        cache: "no-store",
        headers: authHeaders,
        signal: AbortSignal.timeout(10_000),
      });
      const heartbeatRes = await fetch(`${base.api}/assets/heartbeat`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          asset_id: assetID,
          type: "connector-cluster",
          name: clusterName || hostLabelFromURL(normalizedBaseURL, "proxmox"),
          source: "proxmox",
          status: "online",
          metadata: {
            connector_type: "proxmox",
            base_url: normalizedBaseURL
          }
        })
      });
      if (!heartbeatRes.ok) {
        const payload = await safeJSON(heartbeatRes);
        return NextResponse.json(payload ?? { error: "failed to create proxmox cluster asset" }, { status: heartbeatRes.status });
      }
      if (existingAssetRes.status === 404) {
        createdAssetID = assetID;
      }

      const credRes = await fetch(`${base.api}/credentials/profiles`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          name: credLabel,
          kind: credKind,
          username: credUsername,
          secret: tokenSecret,
          metadata: {
            base_url: normalizedBaseURL,
            cluster_name: clusterName
          }
        })
      });
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile; error?: string } | null;
      if (!credRes.ok || !credPayload?.profile?.id) {
        await cleanupCreatedProxmoxResources({
          baseAPIURL: base.api,
          authHeaders,
          assetID: createdAssetID,
        });
        return NextResponse.json(
          credPayload ?? { error: "failed to create proxmox credential profile" },
          { status: credRes.status || 500 }
        );
      }
      credentialID = credPayload.profile.id;
      createdCredentialID = credentialID;

      proxmoxConfig.credential_id = credentialID;

      const createRes = await fetch(`${base.api}/hub-collectors`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        signal: AbortSignal.timeout(10_000),
        body: JSON.stringify({
          asset_id: assetID,
          collector_type: "proxmox",
          enabled: true,
          interval_seconds: intervalSeconds,
          config: proxmoxConfig
        })
      });
      const createPayload = await safeJSON(createRes);
      if (!createRes.ok) {
        await cleanupCreatedProxmoxResources({
          baseAPIURL: base.api,
          authHeaders,
          assetID: createdAssetID,
          credentialID: createdCredentialID,
        });
        return NextResponse.json(createPayload ?? { error: "failed to create proxmox collector" }, { status: createRes.status });
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
      { error: error instanceof Error ? error.message : "failed to save proxmox settings" },
      { status: 502 }
    );
  }
}

async function cleanupCreatedProxmoxResources({
  baseAPIURL,
  authHeaders,
  assetID,
  credentialID,
}: {
  baseAPIURL: string;
  authHeaders: HeadersInit;
  assetID?: string;
  credentialID?: string;
}) {
  const cleanupRequests: Promise<Response>[] = [];
  if (credentialID) {
    cleanupRequests.push(fetch(`${baseAPIURL}/credentials/profiles/${encodeURIComponent(credentialID)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(10_000),
    }));
  }
  if (assetID) {
    cleanupRequests.push(fetch(`${baseAPIURL}/assets/${encodeURIComponent(assetID)}`, {
      method: "DELETE",
      cache: "no-store",
      headers: authHeaders,
      signal: AbortSignal.timeout(10_000),
    }));
  }
  if (cleanupRequests.length > 0) {
    await Promise.allSettled(cleanupRequests);
  }
}
