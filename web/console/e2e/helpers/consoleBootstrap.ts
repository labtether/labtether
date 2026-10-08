import { type Page } from "@playwright/test";

export function buildRuntimeSettingsPayload(overrides: Record<string, string>) {
  const settings = [
    {
      key: "console.poll_interval_seconds",
      label: "Status Poll Interval",
      description: "Dashboard status refresh interval in seconds.",
      scope: "console",
      type: "int",
      env_var: "LABTETHER_POLL_INTERVAL_SECONDS",
      default_value: "5",
      env_value: "5"
    },
    {
      key: "console.default_telemetry_window",
      label: "Default Telemetry Window",
      description: "Default telemetry range selection.",
      scope: "console",
      type: "enum",
      env_var: "LABTETHER_DEFAULT_TELEMETRY_WINDOW",
      default_value: "1h",
      env_value: "1h",
      allowed_values: ["15m", "1h", "6h", "24h"]
    },
    {
      key: "console.default_log_window",
      label: "Default Logs Window",
      description: "Default log query range selection.",
      scope: "console",
      type: "enum",
      env_var: "LABTETHER_DEFAULT_LOG_WINDOW",
      default_value: "1h",
      env_value: "1h",
      allowed_values: ["15m", "1h", "6h", "24h"]
    },
    {
      key: "console.log_query_limit",
      label: "Log Query Limit",
      description: "Maximum events requested per log query.",
      scope: "console",
      type: "int",
      env_var: "LABTETHER_LOG_QUERY_LIMIT",
      default_value: "120",
      env_value: "120"
    },
    {
      key: "console.default_actor_id",
      label: "Default Actor ID",
      description: "Default actor identity used for command and action requests.",
      scope: "console",
      type: "string",
      env_var: "LABTETHER_DEFAULT_ACTOR_ID",
      default_value: "owner",
      env_value: "owner"
    },
    {
      key: "console.default_action_dry_run",
      label: "Default Action Dry Run",
      description: "Default dry-run mode for connector actions.",
      scope: "console",
      type: "bool",
      env_var: "LABTETHER_DEFAULT_ACTION_DRY_RUN",
      default_value: "true",
      env_value: "true"
    },
    {
      key: "console.default_update_dry_run",
      label: "Default Update Dry Run",
      description: "Default dry-run mode for update plan execution.",
      scope: "console",
      type: "bool",
      env_var: "LABTETHER_DEFAULT_UPDATE_DRY_RUN",
      default_value: "true",
      env_value: "true"
    }
  ].map((entry) => {
    const override = overrides[entry.key];
    const hasOverride = typeof override === "string" && override.trim() !== "";
    return {
      ...entry,
      override_value: hasOverride ? override : undefined,
      effective_value: hasOverride ? override : entry.env_value || entry.default_value,
      source: hasOverride ? "ui" : "docker"
    };
  });

  return {
    settings,
    overrides
  };
}

export async function mockConsoleBootstrap(page: Page) {
  await page.route("**/api/auth/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ user: { id: "owner", username: "admin", role: "owner" } })
    });
  });

  await page.route("**/api/agents/connected", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ assets: [] })
    });
  });

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: "2026-01-01T12:00:00.000Z",
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: 0,
          staleAssetCount: 0
        },
        endpoints: [],
        assets: [],
        telemetryOverview: []
      })
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
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
          staleAssetCount: 0
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
          top_error_classes: []
        },
        sessions: [],
        recentCommands: [],
        recentAudit: []
      })
    });
  });

  await page.route("**/api/settings/enrollment", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ tokens: [], hub_url: "http://127.0.0.1:8080", ws_url: "ws://127.0.0.1:8080/ws/agent" })
    });
  });

  await page.route("**/api/settings/agent-tokens", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ tokens: [] })
    });
  });

  await page.route(/\/api\/services\/web\/compat(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ compatible: [] })
    });
  });
}
