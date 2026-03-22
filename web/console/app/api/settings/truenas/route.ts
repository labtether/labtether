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
      headers: authHeaders
    });
    const collectorsPayload = (await safeJSON(collectorsRes)) as { collectors?: HubCollector[]; error?: string } | null;
    if (!collectorsRes.ok) {
      return NextResponse.json(
        collectorsPayload ?? { error: "failed to load hub collectors" },
        { status: collectorsRes.status }
      );
    }

    const collector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "truenas");
    if (!collector) {
      return NextResponse.json({
        configured: false,
        settings: {
          base_url: "",
          display_name: "",
          skip_verify: false,
          interval_seconds: 60
        }
      });
    }

    const config = collector.config ?? {};
    const credentialID = stringValue(config["credential_id"]);
    let credential: CredentialProfile | null = null;
    if (credentialID) {
      const credRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}`, {
        cache: "no-store",
        headers: authHeaders
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
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60)
      }
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load truenas settings" },
      { status: 502 }
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
  const apiKey = stringValue(body.api_key);
  const displayName = stringValue(body.display_name);
  const skipVerify = boolValue(body.skip_verify, false);
  const intervalSeconds = clampInterval(numberValue(body.interval_seconds, 60));

  if (!baseURL) {
    return NextResponse.json({ error: "base_url is required" }, { status: 400 });
  }

  try {
    const collectorsRes = await fetch(`${base.api}/hub-collectors?enabled=false`, {
      cache: "no-store",
      headers: authHeaders
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
      "truenas",
      normalizedBaseURL,
    );

    let credentialID = stringValue(existingCollector?.config?.["credential_id"]);
    let pendingRotateSecret: string | null = null;
    if (!credentialID) {
      if (!apiKey) {
        return NextResponse.json({ error: "api_key is required for initial setup" }, { status: 400 });
      }
    }

    // For new collectors, create the asset heartbeat first (idempotent) before
    // creating the credential — this avoids orphaned credentials if the
    // heartbeat step fails.
    let assetID: string | undefined;
    if (!existingCollector?.id) {
      const label = displayName || hostLabelFromURL(normalizedBaseURL, "truenas");
      assetID = `truenas-${slugify(label, "truenas")}`;
      const heartbeatRes = await fetch(`${base.api}/assets/heartbeat`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        body: JSON.stringify({
          asset_id: assetID,
          type: "storage-controller",
          name: label,
          source: "truenas",
          status: "online",
          metadata: {
            connector_type: "truenas",
            base_url: normalizedBaseURL
          }
        })
      });
      if (!heartbeatRes.ok) {
        const payload = await safeJSON(heartbeatRes);
        return NextResponse.json(payload ?? { error: "failed to create truenas asset" }, { status: heartbeatRes.status });
      }
    }

    // Create or rotate the credential.
    if (!credentialID) {
      const credName = displayName || hostLabelFromURL(normalizedBaseURL, "truenas");
      const credRes = await fetch(`${base.api}/credentials/profiles`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        body: JSON.stringify({
          name: `TrueNAS API Key (${credName})`,
          kind: "truenas_api_key",
          secret: apiKey,
          metadata: {
            base_url: normalizedBaseURL,
            display_name: displayName
          }
        })
      });
      const credPayload = (await safeJSON(credRes)) as { profile?: CredentialProfile; error?: string } | null;
      if (!credRes.ok || !credPayload?.profile?.id) {
        return NextResponse.json(
          credPayload ?? { error: "failed to create truenas credential profile" },
          { status: credRes.status || 500 }
        );
      }
      credentialID = credPayload.profile.id;
    } else if (apiKey) {
      // Defer rotation until collector update succeeds to avoid stale collector+new secret mismatch.
      pendingRotateSecret = apiKey;
    }

    const truenasConfig = {
      base_url: normalizedBaseURL,
      display_name: displayName,
      credential_id: credentialID,
      skip_verify: skipVerify,
      interval_seconds: intervalSeconds
    };

    let collectorResponsePayload: unknown;
    if (existingCollector?.id) {
      const patchRes = await fetch(`${base.api}/hub-collectors/${encodeURIComponent(existingCollector.id)}`, {
        method: "PATCH",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        body: JSON.stringify({
          enabled: true,
          interval_seconds: intervalSeconds,
          config: truenasConfig
        })
      });
      const patchPayload = await safeJSON(patchRes);
      if (!patchRes.ok) {
        return NextResponse.json(patchPayload ?? { error: "failed to update truenas collector" }, { status: patchRes.status });
      }
      collectorResponsePayload = patchPayload;

      if (pendingRotateSecret) {
        const rotateRes = await fetch(`${base.api}/credentials/profiles/${encodeURIComponent(credentialID)}/rotate`, {
          method: "POST",
          cache: "no-store",
          headers: {
            "Content-Type": "application/json",
            ...authHeaders
          },
          body: JSON.stringify({
            secret: pendingRotateSecret,
            reason: "updated from settings truenas section"
          })
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
      const createRes = await fetch(`${base.api}/hub-collectors`, {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...authHeaders
        },
        body: JSON.stringify({
          asset_id: assetID,
          collector_type: "truenas",
          enabled: true,
          interval_seconds: intervalSeconds,
          config: truenasConfig
        })
      });
      const createPayload = await safeJSON(createRes);
      if (!createRes.ok) {
        return NextResponse.json(createPayload ?? { error: "failed to create truenas collector" }, { status: createRes.status });
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
      { error: error instanceof Error ? error.message : "failed to save truenas settings" },
      { status: 502 }
    );
  }
}
