import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../../lib/backend";
import { isMutationRequestOriginAllowed } from "../../../../lib/proxyAuth";
import { clampInterval, numberValue, safeJSON, slugify, stringValue } from "../shared";

export const dynamic = "force-dynamic";

type HubCollector = {
  id: string;
  asset_id: string;
  collector_type: string;
  enabled: boolean;
  interval_seconds: number;
  config?: Record<string, unknown>;
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

    const collector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "docker");
    if (!collector) {
      return NextResponse.json({
        configured: false,
        settings: {
          cluster_name: "",
          interval_seconds: 60
        }
      });
    }

    const config = collector.config ?? {};
    return NextResponse.json({
      configured: true,
      collector_id: collector.id,
      asset_id: collector.asset_id,
      settings: {
        cluster_name: stringValue(config["cluster_name"]),
        interval_seconds: numberValue(config["interval_seconds"], collector.interval_seconds || 60),
      }
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load docker settings" },
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

  const clusterName = stringValue(body.cluster_name) || "Docker Cluster";
  const intervalSeconds = clampInterval(numberValue(body.interval_seconds, 60));

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
    const existingCollector = (collectorsPayload?.collectors ?? []).find((entry) => entry.collector_type === "docker");

    const dockerConfig = {
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
          config: dockerConfig,
        }),
      });
      const patchPayload = await safeJSON(patchRes);
      if (!patchRes.ok) {
        return NextResponse.json(patchPayload ?? { error: "failed to update docker collector" }, { status: patchRes.status });
      }
      collectorResponsePayload = patchPayload;
    } else {
      const assetID = `docker-cluster-${slugify(clusterName, "docker")}`;
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
          name: clusterName,
          source: "docker",
          status: "online",
          metadata: {
            connector_type: "docker",
            discovered: "0"
          }
        }),
      });
      if (!heartbeatRes.ok) {
        const payload = await safeJSON(heartbeatRes);
        return NextResponse.json(payload ?? { error: "failed to create docker cluster asset" }, { status: heartbeatRes.status });
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
          collector_type: "docker",
          enabled: true,
          interval_seconds: intervalSeconds,
          config: dockerConfig
        }),
      });
      const createPayload = await safeJSON(createRes);
      if (!createRes.ok) {
        return NextResponse.json(createPayload ?? { error: "failed to create docker collector" }, { status: createRes.status });
      }
      collectorResponsePayload = createPayload;
    }

    return NextResponse.json({
      configured: true,
      result: collectorResponsePayload
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to save docker settings" },
      { status: 502 }
    );
  }
}
