import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

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
