import { expect, test, type Page, type Route } from "@playwright/test";

const THEMES = ["oled", "dark", "light"] as const;

const VISUAL_ROUTES = [
  { id: "settings", path: "/settings", heading: "Settings" },
  { id: "alerts", path: "/alerts", heading: "Alerts" },
  { id: "logs", path: "/logs", heading: "Logs" },
  { id: "health", path: "/reliability", heading: "Health" },
  { id: "devices", path: "/nodes", heading: "Devices" },
] as const;

const statusPayload = {
  timestamp: "2026-01-01T12:00:00.000Z",
  summary: {
    servicesUp: 5,
    servicesTotal: 5,
    connectorCount: 0,
    groupCount: 0,
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
  groups: [],
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

const liveStatusPayload = {
  timestamp: "2026-01-01T12:00:00.000Z",
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

const runtimeSettingsPayload = {
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
};

const retentionPayload = {
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
};

async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function installConsoleApiMocks(page: Page, unmocked: Set<string>) {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const { pathname } = url;
    const method = route.request().method();

    if (pathname === "/api/auth/me") {
      await fulfillJSON(route, { user: { id: "owner", username: "admin", role: "owner" } });
      return;
    }

    if (pathname === "/api/auth/users" && method === "GET") {
      await fulfillJSON(route, { users: [] });
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
      await fulfillJSON(route, runtimeSettingsPayload);
      return;
    }

    if (pathname === "/api/settings/retention") {
      await fulfillJSON(route, retentionPayload);
      return;
    }

    if (pathname === "/api/settings/enrollment") {
      await fulfillJSON(route, { tokens: [], hub_url: "http://127.0.0.1:8080", ws_url: "ws://127.0.0.1:8080/ws/agent" });
      return;
    }

    if (pathname === "/api/settings/agent-tokens") {
      await fulfillJSON(route, { tokens: [] });
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

    if (pathname === "/api/alerts/instances") {
      await fulfillJSON(route, { instances: [] });
      return;
    }

    if (pathname === "/api/alerts/rules") {
      await fulfillJSON(route, { rules: [] });
      return;
    }

    if (pathname === "/api/alerts/templates") {
      await fulfillJSON(route, { templates: [] });
      return;
    }

    if (pathname === "/api/alerts/silences") {
      await fulfillJSON(route, { silences: [] });
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

    if (pathname === "/api/agents/connected") {
      await fulfillJSON(route, { assets: [] });
      return;
    }

    if (pathname === "/api/agents/pending") {
      await fulfillJSON(route, { agents: [] });
      return;
    }

    if (pathname === "/api/ws/events") {
      await route.fulfill({
        status: 204,
        body: "",
      });
      return;
    }

    if (/^\/api\/groups\/[^/]+\/timeline$/.test(pathname)) {
      await fulfillJSON(route, {
        group: { id: "group-1", name: "Home", slug: "home", parent_group_id: "", icon: "", sort_order: 0, created_at: "", updated_at: "" },
        window: "24h",
        impact: {
          error_events: 0,
          warn_events: 0,
          failed_actions: 0,
          failed_updates: 0,
          assets_stale: 0,
          assets_offline: 0,
        },
        events: [],
      });
      return;
    }

    if (/^\/api\/groups\/[^/]+\/maintenance-windows(\/[^/]+)?$/.test(pathname)) {
      await fulfillJSON(route, { windows: [] });
      return;
    }

    if (pathname === "/api/services/web") {
      await fulfillJSON(route, { items: [], groups: [], meta: { total: 0 } });
      return;
    }

    if (pathname === "/api/notifications/channels") {
      await fulfillJSON(route, { channels: [] });
      return;
    }

    if (pathname === "/api/version") {
      await fulfillJSON(route, { version: "dev", started_at: "2026-01-01T00:00:00.000Z" });
      return;
    }

    unmocked.add(`${method} ${pathname}`);
    await fulfillJSON(route, { error: `unmocked endpoint: ${method} ${pathname}` }, 404);
  });
}

test.describe("Visual Regression", () => {
  test.beforeEach(async ({ page, browserName }) => {
    test.skip(browserName !== "chromium", "Visual baselines are maintained for chromium only.");
    await page.setViewportSize({ width: 1512, height: 982 });
  });

  for (const theme of THEMES) {
    for (const routeDef of VISUAL_ROUTES) {
      test(`${routeDef.id} (${theme})`, async ({ page }) => {
        const unmockedCalls = new Set<string>();
        await installConsoleApiMocks(page, unmockedCalls);

        await page.addInitScript((nextTheme) => {
          localStorage.setItem("labtether.theme", nextTheme);
          localStorage.setItem("labtether.density", "minimal");
        }, theme);

        await page.goto(routeDef.path);
        await expect(page.getByRole("heading", { name: routeDef.heading, level: 1, exact: true })).toBeVisible();
        await page.waitForTimeout(150);

        await expect(page).toHaveScreenshot(`visual-${routeDef.id}-${theme}.png`, {
          fullPage: true,
          animations: "disabled",
          caret: "hide",
        });

        expect([...unmockedCalls]).toEqual([]);
      });
    }
  }
});
