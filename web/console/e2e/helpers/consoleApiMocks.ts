import { type Page, type Route } from "@playwright/test";

export type MockRouteContext = {
  route: Route;
  method: string;
  pathname: string;
  url: URL;
  requestBody: Record<string, unknown>;
  fulfillJSON: (body: unknown, status?: number) => Promise<void>;
};

export type ConsoleApiMockOptions = {
  authMeStatus?: number;
  authUser?: Record<string, unknown>;
  statusPayload?: Record<string, unknown>;
  liveStatusPayload?: Record<string, unknown>;
  customRoute?: (context: MockRouteContext) => Promise<boolean> | boolean;
  unmocked?: Set<string>;
};

const BASE_TIMESTAMP = "2026-01-01T12:00:00.000Z";

export function buildStatusPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const defaults: Record<string, unknown> = {
    timestamp: BASE_TIMESTAMP,
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 0,
      groupCount: 1,
      assetCount: 0,
      sessionCount: 0,
      auditCount: 0,
      processedJobs: 0,
      actionRunCount: 0,
      updateRunCount: 0,
      deadLetterCount: 0,
      staleAssetCount: 0,
      retentionError: "",
    },
    endpoints: [],
    connectors: [],
    groups: [
      {
        id: "group-1",
        name: "Home",
        slug: "home",
        sort_order: 0,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ],
    assets: [],
    telemetryOverview: [],
    recentLogs: [],
    logSources: [],
    groupReliability: [],
    actionRuns: [],
    updatePlans: [],
    updateRuns: [],
    deadLetters: [],
    deadLetterAnalytics: {
      window: "24h",
      bucket: "1h",
      total: 0,
      rate_per_hour: 0,
      rate_per_day: 0,
      trend: [],
      top_components: [],
      top_subjects: [],
      top_error_classes: [],
    },
    sessions: [],
    recentCommands: [],
    recentAudit: [],
  };

  const merged: Record<string, unknown> = { ...defaults, ...overrides };
  const defaultSummary = defaults.summary as Record<string, unknown>;
  const overrideSummary = (overrides.summary as Record<string, unknown> | undefined) ?? {};
  const summary: Record<string, unknown> = { ...defaultSummary, ...overrideSummary };

  if (overrideSummary.assetCount == null) {
    const assets = Array.isArray(merged.assets) ? merged.assets : [];
    summary.assetCount = assets.length;
  }
  if (overrideSummary.groupCount == null) {
    const groups = Array.isArray(merged.groups) ? merged.groups : [];
    summary.groupCount = groups.length;
  }

  merged.summary = summary;
  return merged;
}

export function buildLiveStatusPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const defaults: Record<string, unknown> = {
    timestamp: BASE_TIMESTAMP,
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      assetCount: 0,
      staleAssetCount: 0,
    },
    endpoints: [],
    assets: [],
    telemetryOverview: [],
  };

  const merged: Record<string, unknown> = { ...defaults, ...overrides };
  const defaultSummary = defaults.summary as Record<string, unknown>;
  const overrideSummary = (overrides.summary as Record<string, unknown> | undefined) ?? {};
  const summary: Record<string, unknown> = { ...defaultSummary, ...overrideSummary };

  if (overrideSummary.assetCount == null) {
    const assets = Array.isArray(merged.assets) ? merged.assets : [];
    summary.assetCount = assets.length;
  }
  merged.summary = summary;
  return merged;
}

async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

function parseRequestBody(route: Route): Record<string, unknown> {
  try {
    const raw = route.request().postData();
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return typeof parsed === "object" && parsed !== null
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}

export async function installConsoleApiMocks(page: Page, options: ConsoleApiMockOptions = {}) {
  const authStatus = options.authMeStatus ?? 200;
  const authUser = options.authUser ?? { id: "owner", username: "admin", role: "owner" };
  const statusPayload = options.statusPayload ?? buildStatusPayload();
  const liveStatusPayload = options.liveStatusPayload ?? buildLiveStatusPayload({
    assets: (statusPayload["assets"] as unknown[]) ?? [],
    telemetryOverview: (statusPayload["telemetryOverview"] as unknown[]) ?? [],
  });

  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const method = route.request().method();
    const pathname = url.pathname;
    const requestBody = parseRequestBody(route);

    const context: MockRouteContext = {
      route,
      method,
      pathname,
      url,
      requestBody,
      fulfillJSON: (body: unknown, status = 200) => fulfillJSON(route, body, status),
    };

    if (options.customRoute && (await options.customRoute(context))) {
      return;
    }

    if (pathname === "/api/auth/me") {
      if (authStatus >= 200 && authStatus < 300) {
        await fulfillJSON(route, { user: authUser }, authStatus);
      } else {
        await fulfillJSON(route, { error: "unauthorized" }, authStatus);
      }
      return;
    }

    if (pathname === "/api/auth/login") {
      await fulfillJSON(route, { user: authUser, ok: true });
      return;
    }

    if (pathname === "/api/auth/bootstrap/status") {
      await fulfillJSON(route, { setup_required: false });
      return;
    }

    if (pathname === "/api/auth/logout") {
      await fulfillJSON(route, { ok: true });
      return;
    }

    if (pathname === "/api/auth/providers") {
      await fulfillJSON(route, { local: { enabled: true }, oidc: { enabled: false } });
      return;
    }

    if (pathname === "/api/settings/managed-database") {
      await fulfillJSON(route, {
        enabled: false,
        host: "",
        port: 5432,
        username: "",
        password: "",
        database: "",
      });
      return;
    }

    if (pathname === "/api/settings/managed-database/reveal") {
      await fulfillJSON(route, { password: "" });
      return;
    }

    if (pathname === "/api/auth/users" && method === "GET") {
      await fulfillJSON(route, { users: [] });
      return;
    }

    if (pathname === "/api/auth/users" && method === "POST") {
      const username = typeof requestBody.username === "string" ? requestBody.username : "new-user";
      const role = typeof requestBody.role === "string" ? requestBody.role : "viewer";
      await fulfillJSON(route, { user: { id: "usr-e2e-001", username, role } }, 201);
      return;
    }

    if (pathname.startsWith("/api/auth/users/") && method === "PATCH") {
      const id = pathname.split("/").pop() || "usr-e2e-001";
      const role = typeof requestBody.role === "string" ? requestBody.role : "viewer";
      await fulfillJSON(route, { user: { id, username: "patched-user", role } });
      return;
    }

    if (pathname === "/api/status") {
      await fulfillJSON(route, statusPayload);
      return;
    }

    if (pathname === "/api/status/live") {
      await fulfillJSON(route, liveStatusPayload);
      return;
    }

    if (pathname === "/api/settings/runtime") {
      await fulfillJSON(route, {
        settings: [
          {
            key: "console.poll_interval_seconds",
            label: "Status Poll Interval",
            description: "Dashboard status refresh interval in seconds.",
            scope: "console",
            type: "int",
            env_var: "LABTETHER_POLL_INTERVAL_SECONDS",
            default_value: "5",
            env_value: "5",
            effective_value: "5",
            source: "docker",
          },
        ],
        overrides: {},
      });
      return;
    }

    if (pathname === "/api/agents/connected") {
      await fulfillJSON(route, { agents: [] });
      return;
    }

    if (pathname === "/api/terminal/snippets") {
      await fulfillJSON(route, []);
      return;
    }

    if (pathname === "/api/topology") {
      await fulfillJSON(route, {
        data: {
          id: "topology-e2e",
          name: "LabTether",
          zones: [],
          members: [],
          connections: [],
          unsorted: [],
          viewport: { x: 0, y: 0, zoom: 1 },
        },
      });
      return;
    }

    if (pathname === "/api/settings/retention") {
      await fulfillJSON(route, {
        settings: {
          logs_window: "14d",
          metrics_window: "7d",
          audit_window: "30d",
          terminal_window: "30d",
          action_runs_window: "60d",
          update_runs_window: "60d",
        },
        presets: [
          {
            id: "balanced",
            name: "Balanced",
            description: "Balanced retention",
            settings: {
              logs_window: "14d",
              metrics_window: "7d",
              audit_window: "30d",
              terminal_window: "30d",
              action_runs_window: "60d",
              update_runs_window: "60d",
            },
          },
        ],
      });
      return;
    }

    if (pathname === "/api/settings/enrollment") {
      await fulfillJSON(route, {
        tokens: [],
        hub_url: "http://127.0.0.1:8080",
        ws_url: "ws://127.0.0.1:8080/ws/agent",
      });
      return;
    }

    if (pathname === "/api/v1/tls/info") {
      await fulfillJSON(route, {
        tls_enabled: true,
        cert_type: "self-signed",
        ca_available: true,
        ca_fingerprint_sha256: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
        ca_expires: "2028-01-01T00:00:00.000Z",
      });
      return;
    }

    if (pathname === "/api/v1/discovery/proposals") {
      await fulfillJSON(route, { proposals: [] });
      return;
    }

    if (pathname === "/api/file-connections" || pathname === "/api/file-connections/") {
      await fulfillJSON(route, { connections: [] });
      return;
    }

    if (pathname === "/api/notifications/channels") {
      await fulfillJSON(route, { channels: [] });
      return;
    }

    if (pathname === "/api/version") {
      await fulfillJSON(route, {
        version: "0.0.0-e2e",
        commit: "e2e",
        built_at: "2026-01-01T00:00:00.000Z",
      });
      return;
    }

    if (pathname === "/api/settings/agent-tokens") {
      await fulfillJSON(route, { tokens: [] });
      return;
    }

    if (pathname === "/api/settings/proxmox") {
      await fulfillJSON(route, {
        configured: false,
        settings: {
          base_url: "",
          auth_method: "api_token",
          token_id: "",
          username: "",
          skip_verify: true,
          ca_pem: "",
          cluster_name: "",
          interval_seconds: 60,
        },
      });
      return;
    }

    if (pathname === "/api/settings/proxmox/test") {
      await fulfillJSON(route, { status: "ok", message: "Proxmox API reachable." });
      return;
    }

    if (pathname === "/api/settings/portainer") {
      await fulfillJSON(route, {
        configured: false,
        settings: {
          base_url: "",
          auth_method: "api_key",
          token_id: "",
          cluster_name: "",
          skip_verify: true,
          interval_seconds: 60,
        },
      });
      return;
    }

    if (pathname === "/api/settings/portainer/test") {
      await fulfillJSON(route, { status: "ok", message: "Portainer API reachable." });
      return;
    }

    if (pathname === "/api/settings/pbs") {
      await fulfillJSON(route, {
        configured: false,
        settings: {
          base_url: "",
          token_id: "",
          display_name: "",
          skip_verify: true,
          ca_pem: "",
          interval_seconds: 60,
        },
      });
      return;
    }

    if (pathname === "/api/settings/pbs/test") {
      await fulfillJSON(route, { status: "ok", message: "PBS API reachable." });
      return;
    }

    if (pathname === "/api/settings/truenas") {
      await fulfillJSON(route, {
        configured: false,
        settings: {
          base_url: "",
          api_key: "",
          display_name: "",
          skip_verify: true,
          interval_seconds: 60,
        },
      });
      return;
    }

    if (pathname === "/api/settings/truenas/test") {
      await fulfillJSON(route, { status: "ok", message: "TrueNAS API reachable." });
      return;
    }

    if (/^\/api\/settings\/collectors\/[^/]+\/run$/.test(pathname)) {
      await fulfillJSON(route, { message: "Collector run started." });
      return;
    }

    if (/^\/api\/settings\/collectors\/[^/]+$/.test(pathname)) {
      await fulfillJSON(route, {
        collector: {
          id: pathname.split("/").pop() ?? "",
          last_status: "ok",
          last_error: "",
          last_collected_at: BASE_TIMESTAMP,
        },
        discovered: 0,
      });
      return;
    }

    if (pathname === "/api/logs/query") {
      await fulfillJSON(route, { events: [] });
      return;
    }

    if (pathname === "/api/telemetry/perf") {
      await fulfillJSON(route, { accepted: true }, 202);
      return;
    }

    if (pathname === "/api/logs/sources") {
      await fulfillJSON(route, { sources: [] });
      return;
    }

    if (pathname === "/api/alerts/instances") {
      if (method === "POST") {
        await fulfillJSON(route, { ok: true });
      } else {
        await fulfillJSON(route, { instances: [] });
      }
      return;
    }

    if (pathname === "/api/alerts/rules") {
      if (method === "POST") {
        await fulfillJSON(route, { ok: true });
      } else {
        await fulfillJSON(route, { rules: [] });
      }
      return;
    }

    if (pathname === "/api/alerts/templates") {
      await fulfillJSON(route, { templates: [] });
      return;
    }

    if (/^\/api\/alerts\/rules\/[^/]+$/.test(pathname) && method === "DELETE") {
      await fulfillJSON(route, { ok: true });
      return;
    }

    if (pathname === "/api/alerts/silences") {
      if (method === "POST") {
        await fulfillJSON(route, { ok: true });
      } else {
        await fulfillJSON(route, { silences: [] });
      }
      return;
    }

    if (/^\/api\/alerts\/silences\/[^/]+$/.test(pathname) && method === "DELETE") {
      await fulfillJSON(route, { ok: true });
      return;
    }

    if (pathname === "/api/agents/connected") {
      await fulfillJSON(route, { assets: [] });
      return;
    }

    if (pathname === "/api/agents/pending") {
      await fulfillJSON(route, { agents: [] });
      return;
    }

    if (pathname === "/api/agents/approve" || pathname === "/api/agents/reject") {
      await fulfillJSON(route, { ok: true });
      return;
    }

    if (pathname === "/api/links/suggestions") {
      await fulfillJSON(route, { suggestions: [] });
      return;
    }

    if (/^\/api\/links\/suggestions\/[^/]+$/.test(pathname) && method === "PUT") {
      await fulfillJSON(route, { ok: true });
      return;
    }

    if (pathname === "/api/actions/execute") {
      await fulfillJSON(route, { ok: true, run_id: "run-1", queued: true, request: requestBody });
      return;
    }

    if (pathname === "/api/services/web/icon-library") {
      if (method === "POST") {
        await fulfillJSON(route, {
          icon: {
            id: "icon-1",
            name: "Uploaded icon",
            data_url: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=",
          },
        });
      } else if (method === "PATCH") {
        await fulfillJSON(route, {
          icon: {
            id: "icon-1",
            name: "Renamed icon",
            data_url: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=",
          },
        });
      } else if (method === "DELETE") {
        await fulfillJSON(route, { ok: true });
      } else {
        await fulfillJSON(route, { icons: [] });
      }
      return;
    }

    if (/^\/api\/groups\/[^/]+\/timeline$/.test(pathname)) {
      await fulfillJSON(route, {
        generated_at: BASE_TIMESTAMP,
        from: "2026-01-01T00:00:00.000Z",
        to: BASE_TIMESTAMP,
        window: "24h",
        group: {
          id: "group-1",
          name: "Home",
          slug: "home",
          sort_order: 0,
          created_at: BASE_TIMESTAMP,
          updated_at: BASE_TIMESTAMP,
        },
        impact: {
          total_events: 0,
          error_events: 0,
          warn_events: 0,
          info_events: 0,
          failed_actions: 0,
          failed_updates: 0,
          assets_stale: 0,
          assets_offline: 0,
          dead_letters: 0,
        },
        reliability: {
          group: {
            id: "group-1",
            name: "Home",
            slug: "home",
            sort_order: 0,
            created_at: BASE_TIMESTAMP,
            updated_at: BASE_TIMESTAMP,
          },
          score: 100,
          grade: "A",
          assets_total: 0,
          assets_online: 0,
          assets_stale: 0,
          assets_offline: 0,
          failed_actions: 0,
          failed_updates: 0,
          error_logs: 0,
          warn_logs: 0,
          dead_letters: 0,
          maintenance_active: false,
          suppress_alerts: false,
          block_actions: false,
          block_updates: false,
        },
        events: [],
      });
      return;
    }

    if (/^\/api\/groups\/[^/]+\/maintenance-windows$/.test(pathname)) {
      if (method === "POST") {
        await fulfillJSON(route, { id: "window-1" });
      } else {
        await fulfillJSON(route, { windows: [] });
      }
      return;
    }

    if (/^\/api\/groups\/[^/]+\/maintenance-windows\/[^/]+$/.test(pathname) && method === "DELETE") {
      await fulfillJSON(route, { ok: true });
      return;
    }

    if (pathname === "/api/ws/events") {
      await route.fulfill({ status: 204, body: "" });
      return;
    }

    if (/^\/api\/pbs\/assets\/[^/]+\/details$/.test(pathname)) {
      await fulfillJSON(route, { tasks: [], kind: "server", node: "localhost", collector_id: "collector-pbs-1" });
      return;
    }

    if (/^\/api\/pbs\/tasks\/[^/]+\/[^/]+\/status$/.test(pathname)) {
      await fulfillJSON(route, { task: { status: "stopped", exitstatus: "OK" } });
      return;
    }

    if (/^\/api\/pbs\/tasks\/[^/]+\/[^/]+\/log$/.test(pathname)) {
      await fulfillJSON(route, { lines: [] });
      return;
    }

    if (/^\/api\/pbs\/tasks\/[^/]+\/[^/]+\/stop$/.test(pathname)) {
      await fulfillJSON(route, { ok: true });
      return;
    }

    options.unmocked?.add(`${method} ${pathname}`);
    await fulfillJSON(route, { error: `unmocked endpoint: ${method} ${pathname}` }, 404);
  });
}
