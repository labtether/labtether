import { expect,test } from "@playwright/test";
import { buildRuntimeSettingsPayload,mockConsoleBootstrap } from './helpers/consoleBootstrap';

test("settings page saves and resets runtime overrides", async ({ page }) => {
  await mockConsoleBootstrap(page);

  let runtimeOverrides: Record<string, string> = {};
  let retentionSettings = {
    logs_window: "14d",
    metrics_window: "7d",
    audit_window: "30d",
    terminal_window: "30d",
    action_runs_window: "60d",
    update_runs_window: "60d"
  };

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date().toISOString(),
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
        actionRuns: [],
        updatePlans: [],
        updateRuns: [],
        deadLetters: [],
        sessions: [],
        recentCommands: [],
        recentAudit: []
      })
    });
  });

  await page.route("**/api/settings/runtime", async (route) => {
    const method = route.request().method();
    if (method === "PATCH") {
      const payload = JSON.parse(route.request().postData() || "{}") as { values?: Record<string, string> };
      runtimeOverrides = {
        ...runtimeOverrides,
        ...(payload.values ?? {})
      };
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(buildRuntimeSettingsPayload(runtimeOverrides))
    });
  });

  await page.route("**/api/settings/runtime/reset", async (route) => {
    const payload = JSON.parse(route.request().postData() || "{}") as { keys?: string[] };
    for (const key of payload.keys ?? []) {
      delete runtimeOverrides[key];
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(buildRuntimeSettingsPayload(runtimeOverrides))
    });
  });

  await page.route("**/api/settings/retention", async (route) => {
    if (route.request().method() === "POST") {
      const payload = JSON.parse(route.request().postData() || "{}") as Record<string, string>;
      if (payload.preset === "balanced") {
        retentionSettings = {
          logs_window: "14d",
          metrics_window: "7d",
          audit_window: "30d",
          terminal_window: "30d",
          action_runs_window: "60d",
          update_runs_window: "60d"
        };
      } else {
        retentionSettings = {
          ...retentionSettings,
          ...payload
        };
      }
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        settings: retentionSettings,
        presets: [
          {
            id: "balanced",
            name: "Balanced",
            description: "Balanced retention",
            settings: retentionSettings
          }
        ]
      })
    });
  });

  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();

  // Advanced (behind Show): every runtime setting is a row named by its key; numbers get a narrow box, text a wide one.
  const advanced = page.locator("#advanced");
  await advanced.getByRole("button", { name: "Show", exact: true }).click();
  const pollRow = advanced.locator('[data-setting-key="console.poll_interval_seconds"]');
  const pollInput = pollRow.locator("input[type='number']");
  const actorInput = advanced.locator('[data-setting-key="console.default_actor_id"] input');
  expect(await pollInput.evaluate((node) => window.getComputedStyle(node).width)).toBe("96px");
  expect(await actorInput.evaluate((node) => window.getComputedStyle(node).width)).toBe("256px");

  await pollInput.fill("8");
  await advanced.getByRole("button", { name: "Save", exact: true }).click();

  await expect(advanced.getByText("Runtime settings saved.")).toBeVisible();
  await expect(pollInput).toHaveValue("8");
  await expect(pollRow.getByText("Set here", { exact: true })).toBeVisible();

  await pollRow.getByRole("button", { name: "Reset Status Poll Interval", exact: true }).click();
  await expect(advanced.getByText("Runtime setting reset to Docker/default baseline.")).toBeVisible();
  await expect(pollInput).toHaveValue("5");
});
