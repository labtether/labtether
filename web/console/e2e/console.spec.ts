import { expect, test, type Page } from "@playwright/test";

test("theme and layout toggles persist", async ({ page }) => {
  await mockConsoleBootstrap(page);
  await page.goto("/settings", { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Dark" }).click();
  await expect(page.locator("body")).toHaveAttribute("data-theme", "dark");

  await page.getByRole("button", { name: "Diagnostic" }).click();
  await expect(page.locator("body")).toHaveAttribute("data-density", "diagnostic");

  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.locator("body")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("body")).toHaveAttribute("data-density", "diagnostic");
});

test("terminal route stays on terminal workspace", async ({ page }) => {
  await mockConsoleBootstrap(page);

  await page.goto("/terminal", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL(/\/terminal$/);
  await expect(page.getByRole("button", { name: "New tab", exact: true })).toBeVisible();
});

test("nodes page renders canonical kind labels when available", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "truenas-host-omega",
      name: "OmegaNAS",
      type: "nas",
      source: "truenas",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        hostname: "OmegaNAS"
      },
      resource_class: "storage",
      resource_kind: "storage-controller",
      attributes: {
        source: "truenas",
        hostname: "OmegaNAS"
      }
    }
  ];

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0
        },
        endpoints: [],
        assets,
        telemetryOverview: []
      })
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 1,
          groupCount: 0,
          assetCount: assets.length,
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
        assets,
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

  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  const row = page
    .locator("[role='link']")
    .filter({ has: page.getByText("OmegaNAS", { exact: true }) })
    .first();
  await expect(row).toContainText("Storage Controller");
});

test("nodes page shows populated Home Assistant detail panels and keeps entity grouping collector-scoped", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "ha-hub-main",
      name: "Home Assistant Main",
      type: "connector-cluster",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        connector_type: "homeassistant",
        collector_id: "collector-ha-main",
        collector_base_url: "http://ha-main.local:8123",
        discovered: "2",
      },
    },
    {
      id: "ha-entity-light-kitchen",
      name: "Kitchen Light",
      type: "ha-entity",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        collector_id: "collector-ha-main",
        entity_id: "light.kitchen",
        domain: "light",
        state: "on",
        friendly_name: "Kitchen Light",
        supported_features: "1",
        last_changed: "2026-03-09T08:00:00Z",
        last_updated: "2026-03-09T08:05:00Z",
      },
    },
    {
      id: "ha-entity-sensor-office-temp",
      name: "Office Temp",
      type: "ha-entity",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        collector_id: "collector-ha-main",
        entity_id: "sensor.office_temp",
        domain: "sensor",
        state: "23",
        unit_of_measurement: "C",
        device_class: "temperature",
        state_class: "measurement",
        entity_category: "diagnostic",
        last_changed: "2026-03-09T08:10:00Z",
        last_updated: "2026-03-09T08:12:00Z",
      },
    },
    {
      id: "ha-hub-secondary",
      name: "Home Assistant Secondary",
      type: "connector-cluster",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        connector_type: "homeassistant",
        collector_id: "collector-ha-secondary",
        collector_base_url: "http://ha-secondary.local:8123",
        discovered: "1",
      },
    },
    {
      id: "ha-entity-switch-garden",
      name: "Garden Switch",
      type: "ha-entity",
      source: "homeassistant",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        collector_id: "collector-ha-secondary",
        entity_id: "switch.garden",
        domain: "switch",
        state: "off",
      },
    },
  ];

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets,
        telemetryOverview: [],
      }),
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 1,
          groupCount: 0,
          assetCount: assets.length,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets,
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
      }),
    });
  });

  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  const homeAssistantCard = page
    .locator("[role='link']")
    .filter({ hasText: "Home Assistant Main" })
    .first();

  await expect(homeAssistantCard).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Kitchen Light" })).toHaveCount(0);

  await Promise.all([
    page.waitForURL(/(?:\/[a-z]{2})?\/nodes\/ha-hub-main$/, { timeout: 10_000 }),
    homeAssistantCard.click(),
  ]);

  const homeAssistantPanel = page.getByRole("button", { name: /Home Assistant/i }).first();
  const servicesPanel = page.getByRole("button", { name: /Services/i }).first();
  await expect(homeAssistantPanel).toBeVisible();
  await expect(servicesPanel).toContainText("Services");
  await expect(servicesPanel).toContainText("2 items");
  await expect(page.getByRole("button", { name: /Monitoring/i })).toHaveCount(0);

  await homeAssistantPanel.click();
  await expect(page.getByRole("heading", { name: "Home Assistant Hub", exact: true })).toBeVisible();
  await expect(page.getByText("http://ha-main.local:8123", { exact: true })).toBeVisible();
  await expect(page.getByText("Kitchen Light", { exact: true })).toBeVisible();
  await expect(page.getByText("Office Temp", { exact: true })).toBeVisible();
  await expect(page.getByText("Garden Switch", { exact: true })).toHaveCount(0);

  await page.goto("/nodes/ha-hub-main?panel=services");
  await expect(page.getByRole("link", { name: "Kitchen Light" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Office Temp" })).toBeVisible();
  await expect(page.getByText("Garden Switch", { exact: true })).toHaveCount(0);

  await page.getByRole("link", { name: "Kitchen Light" }).click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/ha-entity-light-kitchen$/);
  await expect(page.getByRole("button", { name: /Home Assistant/i }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: /Monitoring/i })).toHaveCount(0);

  await page.getByRole("button", { name: /Home Assistant/i }).first().click();
  await expect(page.getByRole("heading", { name: "Home Assistant Entity", exact: true })).toBeVisible();
  await expect(page.getByText("Current State", { exact: true })).toBeVisible();
  await expect(page.getByText("light.kitchen", { exact: true })).toBeVisible();
  await expect(page.getByText("Home Assistant Main", { exact: true })).toBeVisible();
});

test("node detail shows remote view for connected agent host", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "agent-host-1",
      name: "Lab Host",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: now,
      metadata: {
        hostname: "lab-host",
      },
    },
  ];

  await page.route("**/api/agents/connected", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ assets: ["agent-host-1"] }),
    });
  });

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets,
        telemetryOverview: [],
      }),
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 0,
          groupCount: 0,
          assetCount: assets.length,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets,
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
      }),
    });
  });

  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await page.locator("[role='link']").filter({ hasText: "Lab Host" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/agent-host-1$/);
  await expect(page.getByText("Lab Host", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /Remote View/i }).first()).toBeVisible();
});

test("portainer node detail shows endpoint-scoped inventory and workload details", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "portainer-endpoint-1",
      name: "Lab Portainer",
      type: "container-host",
      source: "portainer",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        endpoint_id: "1",
        portainer_endpoint_name: "local",
        url: "unix:///var/run/docker.sock",
        type: "docker",
        status: "up",
        portainer_container_count: "1",
        portainer_stack_count: "1",
        portainer_version: "2.21.5",
      },
    },
    {
      id: "portainer-container-1-abc123def456",
      name: "nginx",
      type: "container",
      source: "portainer",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        endpoint_id: "1",
        container_id: "abc123def456789012345678",
        image: "nginx:latest",
        state: "running",
        status: "Up 4 hours",
        stack: "web",
        ports: "8080->80/tcp",
        created_at: "2026-03-09T10:00:00Z",
        labels_json: "{\"app\":\"frontend\"}",
      },
    },
    {
      id: "portainer-stack-10",
      name: "web",
      type: "stack",
      source: "portainer",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        endpoint_id: "1",
        stack_id: "10",
        status: "active",
        type: "compose",
        entry_point: "compose.yml",
        created_by: "admin",
        git_url: "https://github.com/example/web.git",
        portainer_stack_container_count: "1",
      },
    },
    {
      id: "portainer-endpoint-2",
      name: "Edge Portainer",
      type: "container-host",
      source: "portainer",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        endpoint_id: "2",
        url: "tcp://edge:2375",
        type: "docker",
        status: "up",
        portainer_container_count: "1",
        portainer_stack_count: "1",
      },
    },
    {
      id: "portainer-container-2-fff111222333",
      name: "redis",
      type: "container",
      source: "portainer",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        endpoint_id: "2",
        container_id: "fff111222333444555666777",
        image: "redis:7",
        state: "running",
        status: "Up 2 hours",
        stack: "cache",
      },
    },
    {
      id: "portainer-stack-20",
      name: "cache",
      type: "stack",
      source: "portainer",
      status: "online",
      platform: "other",
      last_seen_at: now,
      metadata: {
        endpoint_id: "2",
        stack_id: "20",
        status: "active",
        type: "compose",
        portainer_stack_container_count: "1",
      },
    },
  ];

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets,
        telemetryOverview: [],
      }),
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 1,
          groupCount: 0,
          assetCount: assets.length,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets,
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
      }),
    });
  });

  await page.route(/\/api\/portainer\/assets\/portainer-endpoint-1\/capabilities$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        tabs: ["overview", "containers", "stacks", "images", "volumes", "networks"],
        kind: "host",
        can_exec: true,
        fetched_at: now,
      }),
    });
  });

  await page.route(/\/api\/portainer\/assets\/portainer-endpoint-1\/containers$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: [
          {
            Id: "abc123def456789012345678",
            Names: ["/nginx"],
            Image: "nginx:latest",
            State: "running",
            Status: "Up 4 hours",
            Ports: [{ PrivatePort: 80, PublicPort: 8080, Type: "tcp" }],
          },
        ],
        fetched_at: now,
      }),
    });
  });

  await page.route(/\/api\/portainer\/assets\/portainer-endpoint-1\/stacks$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: [{ Id: 10, Name: "web", Status: 1 }],
        fetched_at: now,
      }),
    });
  });

  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await page.locator("[role='link']").filter({ hasText: "Lab Portainer" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/portainer-endpoint-1$/);
  await expect(page.getByRole("button", { name: /^Docker$/i })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Portainer/i }).first()).toBeVisible();

  await page.getByRole("button", { name: /Portainer/i }).first().click();
  await page.getByRole("button", { name: "Containers", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Containers", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "nginx", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "nginx:latest", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "8080:80/tcp", exact: true })).toBeVisible();
  await expect(page.getByText("redis", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Stacks", exact: true }).click();
  await expect(page.getByRole("cell", { name: "web", exact: true })).toBeVisible();

  await page.getByLabel("Back to dashboard").click();
  await page.getByRole("button", { name: /Compute/i }).first().click();
  await expect(page.getByRole("link", { name: "nginx", exact: true })).toBeVisible();
  await expect(page.getByText("redis", { exact: true })).toHaveCount(0);

  await page.getByRole("link", { name: "nginx", exact: true }).click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/portainer-container-1-abc123def456$/);
  await page.getByRole("button", { name: /Portainer/i }).first().click();
  await expect(page.getByText("nginx:latest", { exact: true })).toBeVisible();
  await expect(page.getByText("8080->80/tcp", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Lab Portainer", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "web", exact: true })).toBeVisible();

  await page.getByRole("link", { name: "Lab Portainer", exact: true }).click();
  await page.goto("/nodes/portainer-stack-10", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/portainer-stack-10$/);
  await page.getByRole("button", { name: /Portainer/i }).first().click();
  await expect(page.getByRole("heading", { name: "Member Containers", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "nginx", exact: true })).toBeVisible();
  await expect(page.getByText("redis", { exact: true })).toHaveCount(0);
});

test("node metrics tolerates telemetry series with null points", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const asset = {
    id: "docker-ct-1",
    name: "App Container",
    type: "docker-container",
    source: "docker",
    status: "online",
    platform: "linux",
    last_seen_at: now,
    metadata: {
      hostname: "app-container",
    },
  };

  const pageErrors: string[] = [];
  page.on("pageerror", (error) => {
    pageErrors.push(error.message);
  });

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: 1,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets: [asset],
        telemetryOverview: [],
      }),
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 0,
          groupCount: 0,
          assetCount: 1,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets: [asset],
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
      }),
    });
  });

  await page.route(/\/api\/metrics\/assets\/docker-ct-1(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        asset: {
          id: asset.id,
          name: asset.name,
          type: asset.type,
          source: asset.source,
          status: asset.status,
          platform: asset.platform,
          last_seen_at: asset.last_seen_at,
        },
        window: "1h",
        step: "1m",
        from: "2026-03-08T00:00:00.000Z",
        to: "2026-03-08T01:00:00.000Z",
        series: [
          {
            metric: "cpu_used_percent",
            unit: "percent",
            points: null,
            current: 62,
          },
          {
            metric: "memory_used_percent",
            unit: "percent",
            points: [
              { ts: 1_741_392_000, value: 41 },
              { ts: 1_741_392_060, value: 44 },
            ],
            current: 44,
          },
        ],
      }),
    });
  });

  await page.goto("/nodes/docker-ct-1?panel=monitoring", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Metrics History", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "CPU Usage", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Memory Usage", exact: true })).toBeVisible();
  await expect(page.getByText("No data points in this window.", { exact: true })).toBeVisible();
  expect(pageErrors).toEqual([]);
});

test("docker stack detail tolerates null container id lists", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const stackAsset = {
    id: "docker-stack-agent-host-1-app",
    name: "App Stack",
    type: "compose-stack",
    source: "docker",
    status: "online",
    platform: "linux",
    last_seen_at: now,
    metadata: {
      agent_id: "agent-host-1",
    },
  };

  const pageErrors: string[] = [];
  page.on("pageerror", (error) => {
    pageErrors.push(error.message);
  });

  await page.route("**/api/status/live", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: 1,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets: [stackAsset],
        telemetryOverview: [],
      }),
    });
  });

  await page.route("**/api/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 0,
          groupCount: 0,
          assetCount: 1,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets: [stackAsset],
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
      }),
    });
  });

  await page.route("**/api/docker/hosts/agent-host-1/stacks", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        stacks: [
          {
            name: "App Stack",
            status: "running",
            config_file: "/opt/app-stack/compose.yaml",
            container_ids: null,
          },
        ],
      }),
    });
  });

  await page.route("**/api/docker/hosts/agent-host-1/containers", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        containers: [
          {
            id: "abcdef123456",
            name: "app",
            image: "ghcr.io/example/app:latest",
            state: "running",
            status: "Up 2 minutes",
            created: now,
            ports: "",
            stack_name: "App Stack",
            labels: null,
          },
        ],
      }),
    });
  });

  await page.goto("/nodes/docker-stack-agent-host-1-app", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Compose Stack", { exact: true })).toBeVisible();
  await expect(page.getByText("/opt/app-stack/compose.yaml", { exact: true })).toBeVisible();
  expect(pageErrors).toEqual([]);
});

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
  await page.getByRole("button", { name: "Advanced", exact: true }).click();

  const advancedSettings = page
    .locator("div")
    .filter({ has: page.getByRole("heading", { name: "Advanced Settings", level: 2, exact: true }) })
    .first();
  const pollRow = advancedSettings
    .getByRole("button", { name: "Setting key: console.poll_interval_seconds", exact: true })
    .locator("xpath=ancestor::div[contains(@class,'grid')][1]");
  const pollInput = pollRow.locator("input[type='number']").first();
  const actorRow = advancedSettings
    .getByRole("button", { name: "Setting key: console.default_actor_id", exact: true })
    .locator("xpath=ancestor::div[contains(@class,'grid')][1]");
  const actorInput = actorRow.locator("input").first();
  const pollWidth = await pollInput.evaluate((node) => window.getComputedStyle(node).width);
  const actorWidth = await actorInput.evaluate((node) => window.getComputedStyle(node).width);

  expect(pollWidth).toBe("96px");
  expect(actorWidth).toBe("256px");

  await pollInput.fill("8");
  await page.getByRole("button", { name: "Save Advanced Settings", exact: true }).click();

  await expect(page.getByText("Runtime settings saved.")).toBeVisible();
  await expect(pollInput).toHaveValue("8");

  await pollRow.getByRole("button", { name: "Reset", exact: true }).click();
  await expect(page.getByText("Runtime setting reset to Docker/default baseline.")).toBeVisible();
  await expect(pollInput).toHaveValue("5");
});

test("add device agent linux installer enables install-vnc-prereqs by default", async ({ page }) => {
  await mockConsoleBootstrap(page);

  await page.route("**/api/settings/enrollment", async (route) => {
    const method = route.request().method();
    if (method === "POST") {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          token: { id: "tok-1" },
          raw_token: "enroll-token-123",
        }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ tokens: [], hub_url: "http://127.0.0.1:8080", ws_url: "ws://127.0.0.1:8080/ws/agent" }),
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /^Agent/i }).first().click();

  await expect(page.getByText("Install Agent", { exact: true })).toBeVisible();
  await expect(page.locator("pre").filter({ hasText: "--install-vnc-prereqs" }).first()).toBeVisible();

  await page.getByRole("button", { name: /Advanced settings/i }).click();
  await page.getByLabel("Enable desktop prerequisites for VNC/remote view").uncheck();
  await expect(page.locator("pre").filter({ hasText: "--install-vnc-prereqs" })).toHaveCount(0);
});

test("add device portainer flow tests and saves connector settings", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const collectorID = "collector-portainer-1";
  let savedSettings: Record<string, unknown> | null = null;
  let lastTestPayload: Record<string, unknown> | null = null;
  let lastSavePayload: Record<string, unknown> | null = null;

  await page.route("**/api/settings/portainer", async (route) => {
    if (route.request().method() === "POST") {
      lastSavePayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
      savedSettings = { ...lastSavePayload };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: true,
          collector_id: collectorID,
          credential_id: "cred-portainer-1",
          result: { collector: { id: collectorID } }
        })
      });
      return;
    }

    if (!savedSettings) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: false,
          settings: {
            base_url: "",
            auth_method: "api_key",
            token_id: "",
            cluster_name: "",
            skip_verify: true,
            interval_seconds: 60
          }
        })
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: true,
        collector_id: collectorID,
        credential_id: "cred-portainer-1",
        settings: {
          base_url: String(savedSettings.base_url ?? ""),
          auth_method: "api_key",
          token_id: String(savedSettings.token_id ?? ""),
          cluster_name: String(savedSettings.cluster_name ?? ""),
          skip_verify: Boolean(savedSettings.skip_verify ?? true),
          interval_seconds: Number(savedSettings.interval_seconds ?? 60)
        }
      })
    });
  });

  await page.route("**/api/settings/portainer/test", async (route) => {
    lastTestPayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ status: "ok", message: "Portainer API reachable." })
    });
  });

  await page.route("**/api/settings/collectors/**", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({})
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        collector: {
          id: collectorID,
          last_status: "ok",
          last_error: "",
          last_collected_at: new Date().toISOString()
        },
        discovered: 3
      })
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Portainer/i }).first().click();

  await expect(page.getByText("Connect Portainer")).toBeVisible();

  const testButton = page.getByRole("button", { name: "Test Connection", exact: true });
  const saveButton = page.getByRole("button", { name: "Save, Sync & Close", exact: true });
  await expect(saveButton).toBeDisabled();

  await page.getByPlaceholder("https://portainer.local:9443").fill("https://portainer.local:9443/");
  await page.getByPlaceholder("Required for initial setup").fill("ptr-secret");

  await expect(saveButton).toBeEnabled();
  await testButton.click();
  await expect(page.getByText("Portainer API reachable.")).toBeVisible();

  await page.getByPlaceholder("Homelab Portainer").fill("Lab Portainer");
  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();

  await expect(page.getByText("Connect Portainer")).toHaveCount(0);
  await expect(page.getByText("Portainer connector saved.")).toBeVisible();

  expect(lastTestPayload).not.toBeNull();
  expect(lastTestPayload).toMatchObject({
    base_url: "https://portainer.local:9443/",
    auth_method: "api_key",
    token_secret: "ptr-secret"
  });

  expect(lastSavePayload).not.toBeNull();
  expect(lastSavePayload).toMatchObject({
    base_url: "https://portainer.local:9443/",
    auth_method: "api_key",
    token_secret: "ptr-secret",
    cluster_name: "Lab Portainer",
    skip_verify: true
  });
});

test("add device portainer flow lets operator choose detected endpoint", async ({ page }) => {
  await mockConsoleBootstrap(page);

  let lastTestPayload: Record<string, unknown> | null = null;

  await page.route(/\/api\/services\/web\/compat(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        compatible: [
          {
            host_asset_id: "asset-a",
            service_id: "service-portainer-a",
            service_name: "Portainer Alpha",
            service_url: "https://alpha-portainer.lan:9443/api",
            connector_id: "portainer",
            confidence: 0.91
          },
          {
            host_asset_id: "asset-a-shadow",
            service_id: "service-portainer-a-shadow",
            service_name: "Portainer Alpha Shadow",
            service_url: "https://alpha-portainer.lan:9443/version",
            connector_id: "portainer",
            confidence: 0.45
          },
          {
            host_asset_id: "asset-b",
            service_id: "service-portainer-b",
            service_name: "Portainer Beta",
            service_url: "https://beta-portainer.lan:9443/api",
            connector_id: "portainer",
            confidence: 0.82
          }
        ]
      })
    });
  });

  await page.route("**/api/settings/portainer", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: false,
        settings: {
          base_url: "",
          auth_method: "api_key",
          token_id: "",
          cluster_name: "",
          skip_verify: true,
          interval_seconds: 60
        }
      })
    });
  });

  await page.route("**/api/settings/portainer/test", async (route) => {
    lastTestPayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ status: "ok", message: "Portainer API reachable." })
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Portainer/i }).first().click();

  const detectedEndpointBlock = page
    .locator("div")
    .filter({ has: page.getByText("Detected Endpoint", { exact: true }) })
    .filter({ has: page.locator("select") })
    .first();
  const detectedEndpointSelect = detectedEndpointBlock.locator("select");

  await expect(detectedEndpointBlock).toBeVisible();
  await expect(detectedEndpointSelect.locator("option")).toHaveCount(2);

  const baseURLInput = page.getByPlaceholder("https://portainer.local:9443");
  const clusterNameInput = page.getByPlaceholder("Homelab Portainer");
  await expect(baseURLInput).toHaveValue("https://alpha-portainer.lan:9443");
  await expect(clusterNameInput).toHaveValue("Portainer Alpha");

  await detectedEndpointSelect.selectOption("https://beta-portainer.lan:9443");
  await expect(baseURLInput).toHaveValue("https://beta-portainer.lan:9443");
  await expect(clusterNameInput).toHaveValue("Portainer Beta");

  await page.getByPlaceholder("Required for initial setup").fill("ptr-secret");
  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("Portainer API reachable.")).toBeVisible();

  expect(lastTestPayload).not.toBeNull();
  expect(lastTestPayload).toMatchObject({
    base_url: "https://beta-portainer.lan:9443",
    token_secret: "ptr-secret"
  });
});

test("add device portainer flow surfaces test-connection failures", async ({ page }) => {
  await mockConsoleBootstrap(page);

  await page.route("**/api/settings/portainer", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: false,
        settings: {
          base_url: "",
          auth_method: "api_key",
          token_id: "",
          cluster_name: "",
          skip_verify: true,
          interval_seconds: 60
        }
      })
    });
  });

  await page.route("**/api/settings/portainer/test", async (route) => {
    await route.fulfill({
      status: 502,
      contentType: "application/json",
      body: JSON.stringify({ error: "portainer test failed" })
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Portainer/i }).first().click();

  await expect(page.getByText("Connect Portainer")).toBeVisible();
  await page.getByPlaceholder("https://portainer.local:9443").fill("https://portainer.local:9443/");

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("portainer test failed")).toBeVisible();
  await expect(page.getByText("Connect Portainer")).toBeVisible();
});

test("add device pbs flow tests and saves connector settings", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const collectorID = "collector-pbs-1";
  let savedSettings: Record<string, unknown> | null = null;
  let lastTestPayload: Record<string, unknown> | null = null;
  let lastSavePayload: Record<string, unknown> | null = null;

  await page.route("**/api/settings/pbs", async (route) => {
    if (route.request().method() === "POST") {
      lastSavePayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
      savedSettings = { ...lastSavePayload };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: true,
          collector_id: collectorID,
          credential_id: "cred-pbs-1",
          result: { collector: { id: collectorID } },
        }),
      });
      return;
    }

    if (!savedSettings) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: false,
          settings: {
            base_url: "",
            token_id: "",
            display_name: "",
            skip_verify: true,
            interval_seconds: 60,
          },
        }),
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: true,
        collector_id: collectorID,
        credential_id: "cred-pbs-1",
        settings: {
          base_url: String(savedSettings.base_url ?? ""),
          token_id: String(savedSettings.token_id ?? ""),
          display_name: String(savedSettings.display_name ?? ""),
          skip_verify: Boolean(savedSettings.skip_verify ?? true),
          interval_seconds: Number(savedSettings.interval_seconds ?? 60),
        },
      }),
    });
  });

  await page.route("**/api/settings/pbs/test", async (route) => {
    lastTestPayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ status: "ok", message: "PBS API reachable." }),
    });
  });

  await page.route("**/api/settings/collectors/**", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({}),
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        collector: {
          id: collectorID,
          last_status: "ok",
          last_error: "",
          last_collected_at: new Date().toISOString(),
        },
        discovered: 4,
      }),
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await expect(page.getByText("Connect PBS")).toBeVisible();

  const testButton = page.getByRole("button", { name: "Test Connection", exact: true });
  const saveButton = page.getByRole("button", { name: "Save, Sync & Close", exact: true });
  await expect(saveButton).toBeDisabled();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");

  await expect(saveButton).toBeEnabled();
  await testButton.click();
  await expect(page.getByText("PBS API reachable.")).toBeVisible();

  await page.getByPlaceholder("Homelab PBS").fill("Lab PBS");
  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();

  await expect(page.getByText("Connect PBS")).toHaveCount(0);
  await expect(page.getByText("PBS connector saved.")).toBeVisible();

  expect(lastTestPayload).not.toBeNull();
  expect(lastTestPayload).toMatchObject({
    base_url: "https://pbs.local:8007/",
    token_id: "root@pam!labtether",
    token_secret: "pbs-secret",
    skip_verify: true,
  });

  expect(lastSavePayload).not.toBeNull();
  expect(lastSavePayload).toMatchObject({
    base_url: "https://pbs.local:8007/",
    token_id: "root@pam!labtether",
    token_secret: "pbs-secret",
    display_name: "Lab PBS",
    skip_verify: true,
  });
});

test("add device pbs flow surfaces test-connection failures", async ({ page }) => {
  await mockConsoleBootstrap(page);

  await page.route("**/api/settings/pbs", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: false,
        settings: {
          base_url: "",
          token_id: "",
          display_name: "",
          skip_verify: true,
          interval_seconds: 60,
        },
      }),
    });
  });

  await page.route("**/api/settings/pbs/test", async (route) => {
    await route.fulfill({
      status: 502,
      contentType: "application/json",
      body: JSON.stringify({ error: "pbs test failed" }),
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await expect(page.getByText("Connect PBS")).toBeVisible();
  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("pbs test failed")).toBeVisible();
  await expect(page.getByText("Connect PBS")).toBeVisible();
});

test("add device truenas flow tests and saves connector settings", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const collectorID = "collector-truenas-1";
  let savedSettings: Record<string, unknown> | null = null;
  let lastTestPayload: Record<string, unknown> | null = null;
  let lastSavePayload: Record<string, unknown> | null = null;

  await page.route("**/api/settings/truenas", async (route) => {
    if (route.request().method() === "POST") {
      lastSavePayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
      savedSettings = { ...lastSavePayload };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: true,
          collector_id: collectorID,
          credential_id: "cred-truenas-1",
          result: { collector: { id: collectorID } }
        })
      });
      return;
    }

    if (!savedSettings) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: false,
          settings: {
            base_url: "",
            display_name: "",
            skip_verify: true,
            interval_seconds: 60
          }
        })
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: true,
        collector_id: collectorID,
        credential_id: "cred-truenas-1",
        settings: {
          base_url: String(savedSettings.base_url ?? ""),
          display_name: String(savedSettings.display_name ?? ""),
          skip_verify: Boolean(savedSettings.skip_verify ?? true),
          interval_seconds: Number(savedSettings.interval_seconds ?? 60)
        }
      })
    });
  });

  await page.route("**/api/settings/truenas/test", async (route) => {
    lastTestPayload = JSON.parse(route.request().postData() || "{}") as Record<string, unknown>;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ status: "ok", message: "TrueNAS API reachable." })
    });
  });

  await page.route("**/api/settings/collectors/**", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({})
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        collector: {
          id: collectorID,
          last_status: "ok",
          last_error: "",
          last_collected_at: new Date().toISOString()
        },
        discovered: 12
      })
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /TrueNAS/i }).first().click();

  await expect(page.getByText("Connect TrueNAS")).toBeVisible();

  const testButton = page.getByRole("button", { name: "Test Connection", exact: true });
  const saveButton = page.getByRole("button", { name: "Save, Sync & Close", exact: true });
  await expect(saveButton).toBeDisabled();

  await page.getByPlaceholder("https://truenas.local").fill("https://omeganas.local:9443/");
  await page.getByPlaceholder("Required").fill("tn-api-key");
  await page.getByPlaceholder("Homelab TrueNAS").fill("OmegaNAS");

  await expect(saveButton).toBeEnabled();
  await testButton.click();
  await expect(page.getByText("TrueNAS API reachable.")).toBeVisible();

  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();

  await expect(page.getByText("Connect TrueNAS")).toHaveCount(0);
  await expect(page.getByText("TrueNAS connector saved.")).toBeVisible();

  expect(lastTestPayload).not.toBeNull();
  expect(lastTestPayload).toMatchObject({
    base_url: "https://omeganas.local:9443/",
    api_key: "tn-api-key",
    skip_verify: true
  });

  expect(lastSavePayload).not.toBeNull();
  expect(lastSavePayload).toMatchObject({
    base_url: "https://omeganas.local:9443/",
    api_key: "tn-api-key",
    display_name: "OmegaNAS",
    skip_verify: true
  });
});

test("dashboard fleet focus surfaces issue-heavy devices", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const groups = [
    { id: "group-lab", name: "Lab", slug: "lab", sort_order: 0, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" },
    { id: "group-garage", name: "Garage", slug: "garage", sort_order: 1, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" }
  ];
  const telemetryOverview = [
    {
      asset_id: "node-offline",
      name: "Lab Edge Offline",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 20 * 60_000).toISOString(),
      metrics: { cpu_used_percent: 15, memory_used_percent: 45, disk_used_percent: 52 }
    },
    {
      asset_id: "node-unresponsive",
      name: "Lab NAS Unresponsive",
      type: "host",
      source: "truenas",
      group_id: "group-garage",
      status: "unresponsive",
      platform: "freebsd",
      last_seen_at: new Date(now - 4 * 60_000).toISOString(),
      metrics: { cpu_used_percent: 40, memory_used_percent: 50, disk_used_percent: 62 }
    },
    {
      asset_id: "node-highload",
      name: "Lab Build Host",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metrics: { cpu_used_percent: 96, memory_used_percent: 82, disk_used_percent: 78 }
    }
  ];
  const assets = [
    {
      id: "node-offline",
      name: "Lab Edge Offline",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 20 * 60_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-unresponsive",
      name: "Lab NAS Unresponsive",
      type: "host",
      source: "truenas",
      group_id: "group-garage",
      status: "unresponsive",
      platform: "freebsd",
      last_seen_at: new Date(now - 4 * 60_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-highload",
      name: "Lab Build Host",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    }
  ];

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 2,
      groupCount: groups.length,
      assetCount: assets.length,
      sessionCount: 0,
      auditCount: 0,
      processedJobs: 0,
      actionRunCount: 0,
      updateRunCount: 0,
      deadLetterCount: 0,
      staleAssetCount: 2
    },
    endpoints: [],
    connectors: [],
    groups,
    assets,
    telemetryOverview,
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 2
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Dashboard", level: 1, exact: true })).toBeVisible();

  await expect(page.getByRole("heading", { name: "Fleet Focus", level: 2, exact: true })).toBeVisible();
  await expect(page.getByText("Lab Edge Offline", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab NAS Unresponsive", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab Build Host", { exact: true }).first()).toBeVisible();
});

test("dashboard topology hero excludes service assets and keeps infrastructure", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const assets = [
    {
      id: "node-host-a",
      name: "Host A",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    },
    {
      id: "svc-api-a",
      name: "svc-a",
      type: "service",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metadata: {}
    }
  ];
  const telemetryOverview = [
    {
      asset_id: "node-host-a",
      name: "Host A",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metrics: { cpu_used_percent: 35, memory_used_percent: 40, disk_used_percent: 42 }
    },
    {
      asset_id: "svc-api-a",
      name: "svc-a",
      type: "service",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metrics: { cpu_used_percent: 0, memory_used_percent: 0, disk_used_percent: 0 }
    }
  ];

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 1,
      groupCount: 0,
      assetCount: assets.length,
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
    assets,
    telemetryOverview,
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.goto("/");
  await expect(page.getByText("Host A", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("svc-a", { exact: true })).toHaveCount(0);
});

test("dashboard topology hero avoids long baseline edges in sparse infra layouts", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const assets = [
    {
      id: "infra-alpha",
      name: "A-Infra",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    },
    {
      id: "infra-bravo",
      name: "B-Infra",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 40_000).toISOString(),
      metadata: {}
    },
    {
      id: "infra-charlie",
      name: "C-Infra",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 50_000).toISOString(),
      metadata: {}
    }
  ];

  const telemetryOverview = assets.map((asset, index) => ({
    asset_id: asset.id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metrics: {
      cpu_used_percent: 20 + index * 5,
      memory_used_percent: 30 + index * 5,
      disk_used_percent: 40 + index * 5
    }
  }));

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 1,
      groupCount: 0,
      assetCount: assets.length,
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
    assets,
    telemetryOverview,
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.goto("/");
  await expect(page.locator(".react-flow__edge-path")).toHaveCount(2);

  const hasLongHorizontalEdge = await page.evaluate(() => {
    const paths = Array.from(document.querySelectorAll<SVGPathElement>(".react-flow__edge-path"));
    return paths.some((path) => {
      const d = path.getAttribute("d");
      if (!d) return false;
      const numbers = d.match(/-?\d*\.?\d+/g)?.map((value) => Number(value)) ?? [];
      if (numbers.length < 4) return false;
      const x1 = numbers[0];
      const y1 = numbers[1];
      const x2 = numbers[numbers.length - 2];
      const y2 = numbers[numbers.length - 1];
      return Math.abs(y2 - y1) < 8 && Math.abs(x2 - x1) > 220;
    });
  });

  expect(hasLongHorizontalEdge).toBeFalsy();
});

test("dashboard fleet focus can expand from issues to full fleet", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const telemetryOverview = [
    {
      asset_id: "node-offline",
      name: "Lab Edge Offline",
      type: "host",
      source: "agent",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 20 * 60_000).toISOString(),
      metrics: { cpu_used_percent: 15, memory_used_percent: 45, disk_used_percent: 52 }
    },
    {
      asset_id: "node-healthy",
      name: "Lab Quiet Host",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metrics: { cpu_used_percent: 24, memory_used_percent: 31, disk_used_percent: 42 }
    }
  ];
  const assets = telemetryOverview.map((asset) => ({
    id: asset.asset_id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metadata: {}
  }));

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 1,
      groupCount: 0,
      assetCount: assets.length,
      sessionCount: 0,
      auditCount: 0,
      processedJobs: 0,
      actionRunCount: 0,
      updateRunCount: 0,
      deadLetterCount: 0,
      staleAssetCount: 1
    },
    endpoints: [],
    connectors: [],
    groups: [],
    assets,
    telemetryOverview,
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 1
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.goto("/");

  const fleetCard = page.locator(".xl\\:col-span-2");

  await expect(fleetCard.locator("a[href$='/nodes/node-offline']")).toHaveCount(1);
  await expect(fleetCard.locator("a[href$='/nodes/node-healthy']")).toHaveCount(0);

  await fleetCard.getByRole("button", { name: "Show all fleet", exact: true }).click();
  await expect(fleetCard.locator("a[href$='/nodes/node-healthy']")).toHaveCount(1);
  await expect(fleetCard.getByRole("button", { name: "Show only issues", exact: true })).toBeVisible();
});

test("dashboard fleet focus shows all devices when no fleet issues exist", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const telemetryOverview = [
    {
      asset_id: "node-alpha",
      name: "Lab Alpha",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metrics: { cpu_used_percent: 32, memory_used_percent: 48, disk_used_percent: 41 }
    },
    {
      asset_id: "node-beta",
      name: "Lab Beta",
      type: "host",
      source: "agent",
      status: "up",
      platform: "linux",
      last_seen_at: new Date(now - 45_000).toISOString(),
      metrics: { cpu_used_percent: 28, memory_used_percent: 36, disk_used_percent: 39 }
    },
    {
      asset_id: "node-gamma",
      name: "Lab Gamma",
      type: "host",
      source: "agent",
      status: "healthy",
      platform: "linux",
      last_seen_at: new Date(now - 60_000).toISOString(),
      metrics: { cpu_used_percent: 44, memory_used_percent: 52, disk_used_percent: 47 }
    }
  ];
  const assets = telemetryOverview.map((asset) => ({
    id: asset.asset_id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metadata: {}
  }));

  const fullStatusPayload = {
    timestamp: new Date(now).toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 1,
      groupCount: 0,
      assetCount: assets.length,
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
    assets,
    telemetryOverview,
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
  };

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date(now).toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload)
    });
  });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Fleet Focus", level: 2, exact: true })).toBeVisible();
  await expect(page.getByText("Lab Alpha", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab Beta", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Lab Gamma", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("No devices found. Add your first device to get started.", { exact: true })).toHaveCount(0);
});

test("nodes page search narrows visible device cards", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = Date.now();
  const groups = [
    { id: "group-lab", name: "Lab", slug: "lab", sort_order: 0, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" },
    { id: "group-garage", name: "Garage", slug: "garage", sort_order: 1, created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z" }
  ];
  const assets: Array<{
    id: string;
    name: string;
    type: string;
    source: string;
    group_id?: string;
    status: string;
    platform: string;
    last_seen_at: string;
    metadata: Record<string, string>;
  }> = [
    {
      id: "node-a",
      name: "Node Alpha",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "online",
      platform: "linux",
      last_seen_at: new Date(now - 30_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-b",
      name: "Node Beta",
      type: "host",
      source: "agent",
      group_id: "group-lab",
      status: "unresponsive",
      platform: "linux",
      last_seen_at: new Date(now - 2 * 60_000).toISOString(),
      metadata: {}
    },
    {
      id: "node-c",
      name: "Node Gamma",
      type: "host",
      source: "agent",
      status: "offline",
      platform: "linux",
      last_seen_at: new Date(now - 15 * 60_000).toISOString(),
      metadata: {}
    }
  ];

  const telemetryOverview = assets.map((asset) => ({
    asset_id: asset.id,
    name: asset.name,
    type: asset.type,
    source: asset.source,
    group_id: asset.group_id,
    status: asset.status,
    platform: asset.platform,
    last_seen_at: asset.last_seen_at,
    metrics: {
      cpu_used_percent: asset.id === "node-a" ? 34 : asset.id === "node-b" ? 71 : 22,
      memory_used_percent: asset.id === "node-a" ? 48 : asset.id === "node-b" ? 76 : 58,
      disk_used_percent: asset.id === "node-a" ? 44 : asset.id === "node-b" ? 73 : 66
    }
  }));

  const fullStatusPayload = () => ({
    timestamp: new Date().toISOString(),
    summary: {
      servicesUp: 5,
      servicesTotal: 5,
      connectorCount: 1,
      groupCount: groups.length,
      assetCount: assets.length,
      sessionCount: 0,
      auditCount: 0,
      processedJobs: 0,
      actionRunCount: 0,
      updateRunCount: 0,
      deadLetterCount: 0,
      staleAssetCount: 2
    },
    endpoints: [],
    connectors: [],
    groups,
    assets,
    telemetryOverview,
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
  });

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: new Date().toISOString(),
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 2
        },
        endpoints: [],
        assets,
        telemetryOverview
      })
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fullStatusPayload())
    });
  });

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  const search = page.getByLabel("Search devices");
  await search.fill("Node Beta");
  await expect(page.locator("[role='link']").filter({ hasText: "Node Beta" })).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Alpha" })).toHaveCount(0);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Gamma" })).toHaveCount(0);

  await search.fill("");
  await page.getByRole("button", { name: "Expand", exact: true }).click();
  await expect(page.locator("[role='link']").filter({ hasText: "Node Alpha" })).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Beta" })).toHaveCount(1);
  await expect(page.locator("[role='link']").filter({ hasText: "Node Gamma" })).toHaveCount(1);
});

test("device card navigation uses a single history entry per click", async ({ page }) => {
  await mockConsoleBootstrap(page);

  const now = new Date().toISOString();
  const assets = [
    {
      id: "node-alpha",
      name: "Node Alpha",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: now,
      metadata: {
        hostname: "node-alpha",
      },
    },
    {
      id: "node-beta",
      name: "Node Beta",
      type: "host",
      source: "agent",
      status: "online",
      platform: "linux",
      last_seen_at: now,
      metadata: {
        hostname: "node-beta",
      },
    },
  ];

  await page.route(/\/api\/status\/live(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          assetCount: assets.length,
          staleAssetCount: 0,
        },
        endpoints: [],
        assets,
        telemetryOverview: [],
      }),
    });
  });

  await page.route(/\/api\/status(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        timestamp: now,
        summary: {
          servicesUp: 5,
          servicesTotal: 5,
          connectorCount: 0,
          groupCount: 0,
          assetCount: assets.length,
          sessionCount: 0,
          auditCount: 0,
          processedJobs: 0,
          actionRunCount: 0,
          updateRunCount: 0,
          deadLetterCount: 0,
          staleAssetCount: 0,
        },
        endpoints: [],
        connectors: [],
        groups: [],
        assets,
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
      }),
    });
  });

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.locator("[role='link']").filter({ hasText: "Node Alpha" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/node-alpha$/);
  await expect(page.getByText("Node Alpha", { exact: true })).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes$/);
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.locator("[role='link']").filter({ hasText: "Node Beta" }).first().click();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes\/node-beta$/);
  await expect(page.getByText("Node Beta", { exact: true })).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/(?:\/[a-z]{2})?\/nodes$/);
});

function buildRuntimeSettingsPayload(overrides: Record<string, string>) {
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

async function mockConsoleBootstrap(page: Page) {
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
