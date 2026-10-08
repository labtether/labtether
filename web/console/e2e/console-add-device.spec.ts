import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

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
  await page.getByLabel("Expected hostname").fill("vnc-test-node");
  await page.getByRole("button", { name: "Create one-time token", exact: true }).click();
  await expect(page.getByText("Enrollment Token", { exact: true })).toBeVisible();
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

  await expect(page.getByText("Connect Portainer", { exact: true })).toBeVisible();

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

  await expect(page.getByText("Connect Portainer", { exact: true })).toHaveCount(0);
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

  await expect(page.getByText("Connect Portainer", { exact: true })).toBeVisible();
  await page.getByPlaceholder("https://portainer.local:9443").fill("https://portainer.local:9443/");

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("portainer test failed")).toBeVisible();
  await expect(page.getByText("Connect Portainer", { exact: true })).toBeVisible();
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
            skip_verify: false,
            ca_pem: "",
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
          skip_verify: Boolean(savedSettings.skip_verify ?? false),
          ca_pem: String(savedSettings.ca_pem ?? ""),
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

  await expect(page.getByText("Connect PBS", { exact: true })).toBeVisible();

  const testButton = page.getByRole("button", { name: "Test Connection", exact: true });
  const saveButton = page.getByRole("button", { name: "Save, Sync & Close", exact: true });
  await expect(saveButton).toBeDisabled();

  await page.getByLabel("Base URL", { exact: true }).fill("https://pbs.local:8007/");
  await page.getByLabel("Token ID", { exact: true }).fill("root@pam!labtether");
  await page.getByLabel("Token Secret", { exact: true }).fill("pbs-secret");
  await page.getByLabel("Custom CA certificate (PEM)", { exact: true }).fill(
    "-----BEGIN CERTIFICATE-----\nqa-ca\n-----END CERTIFICATE-----",
  );

  await expect(saveButton).toBeEnabled();
  await testButton.click();
  await expect(page.getByText("PBS API reachable.")).toBeVisible();

  await page.getByLabel("Display Name", { exact: true }).fill("Lab PBS");
  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();

  await expect(page.getByText("Connect PBS", { exact: true })).toHaveCount(0);
  await expect(page.getByText("PBS connector saved.")).toBeVisible();

  expect(lastTestPayload).not.toBeNull();
  expect(lastTestPayload).toMatchObject({
    base_url: "https://pbs.local:8007/",
    token_id: "root@pam!labtether",
    token_secret: "pbs-secret",
    skip_verify: false,
    ca_pem: "-----BEGIN CERTIFICATE-----\nqa-ca\n-----END CERTIFICATE-----",
  });

  expect(lastSavePayload).not.toBeNull();
  expect(lastSavePayload).toMatchObject({
    base_url: "https://pbs.local:8007/",
    token_id: "root@pam!labtether",
    token_secret: "pbs-secret",
    display_name: "Lab PBS",
    skip_verify: false,
    ca_pem: "-----BEGIN CERTIFICATE-----\nqa-ca\n-----END CERTIFICATE-----",
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

  await expect(page.getByText("Connect PBS", { exact: true })).toBeVisible();
  await page.getByLabel("Base URL", { exact: true }).fill("https://pbs.local:8007/");
  await page.getByLabel("Token ID", { exact: true }).fill("root@pam!labtether");

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("pbs test failed")).toBeVisible();
  await expect(page.getByText("Connect PBS", { exact: true })).toBeVisible();
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

  await expect(page.getByText("Connect TrueNAS", { exact: true })).toBeVisible();

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

  await expect(page.getByText("Connect TrueNAS", { exact: true })).toHaveCount(0);
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
