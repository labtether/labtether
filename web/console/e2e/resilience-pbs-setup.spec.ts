import { expect,test } from "@playwright/test";
import {
installConsoleApiMocks
} from "./helpers/consoleApiMocks";

for (const statusCode of [401, 403, 429, 500]) {
  test(`pbs setup test-connection surfaces backend ${statusCode}`, async ({ page }) => {
    await installConsoleApiMocks(page, {
      customRoute: async ({ pathname, method, fulfillJSON }) => {
        if (pathname === "/api/settings/pbs/test" && method === "POST") {
          await fulfillJSON({ error: `pbs test failed (${statusCode})` }, statusCode);
          return true;
        }
        return false;
      },
    });

    await page.goto("/nodes");
    await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Add Device", exact: true }).click();
    await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

    await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
    await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
    await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");
    await page.getByRole("button", { name: "Test Connection", exact: true }).click();

    await expect(page.getByText(`pbs test failed (${statusCode})`)).toBeVisible();
    await expect(page.getByText("Connect PBS", { exact: true })).toBeVisible();
  });
}

test("pbs setup redacts secret literals in test-connection errors", async ({ page }) => {
  const leakedSecret = "pbs-secret-should-never-render";

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/settings/pbs/test" && method === "POST") {
        await fulfillJSON({ error: `pbs api returned 502: token_secret=${leakedSecret}` }, 502);
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill(leakedSecret);
  await page.getByRole("button", { name: "Test Connection", exact: true }).click();

  await expect(page.getByText("token_secret=[redacted]")).toBeVisible();
  await expect(page.getByText(leakedSecret)).toHaveCount(0);
});

test("pbs setup surfaces offline network failures", async ({ page }) => {
  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, route }) => {
      if (pathname === "/api/settings/pbs/test" && method === "POST") {
        await route.abort("internetdisconnected");
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");
  await page.getByRole("button", { name: "Test Connection", exact: true }).click();

  await expect(
    page.getByText(/ERR_INTERNET_DISCONNECTED|Failed to fetch|NetworkError when attempting to fetch resource|Load failed|failed to test pbs connection/i),
  ).toBeVisible();
});

test("pbs setup save failures keep the wizard open with actionable error", async ({ page }) => {
  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/settings/pbs" && method === "POST") {
        await fulfillJSON({ error: "failed to save pbs settings (500)" }, 500);
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");
  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("failed to save pbs settings (500)")).toBeVisible();
  await expect(dialog.getByText("Connect PBS", { exact: true })).toBeVisible();
});

test("pbs setup recovers after transient test/save failures with retry", async ({ page }) => {
  let testCalls = 0;
  let saveCalls = 0;

  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, route, fulfillJSON }) => {
      if (pathname === "/api/settings/pbs/test" && method === "POST") {
        testCalls++;
        if (testCalls === 1) {
          await route.abort("internetdisconnected");
        } else {
          await fulfillJSON({ status: "ok", message: "pbs API reachable after retry" }, 200);
        }
        return true;
      }
      if (pathname === "/api/settings/pbs" && method === "POST") {
        saveCalls++;
        if (saveCalls === 1) {
          await fulfillJSON({ error: "temporary backend unavailable" }, 503);
        } else {
          await fulfillJSON({ collector_id: "collector-pbs-1" }, 200);
        }
        return true;
      }
      return false;
    },
  });

  await page.goto("/nodes");
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /Proxmox Backup/i }).first().click();

  await page.getByPlaceholder("https://pbs.local:8007").fill("https://pbs.local:8007/");
  await page.getByPlaceholder("root@pam!labtether").fill("root@pam!labtether");
  await page.getByPlaceholder("Required for initial setup").fill("pbs-secret");

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(
    page.getByText(/ERR_INTERNET_DISCONNECTED|Failed to fetch|NetworkError when attempting to fetch resource|Load failed|failed to test pbs connection/i),
  ).toBeVisible();

  await page.getByRole("button", { name: "Test Connection", exact: true }).click();
  await expect(page.getByText("pbs API reachable after retry")).toBeVisible();

  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("temporary backend unavailable")).toBeVisible();
  await expect(dialog.getByText("Connect PBS", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Save, Sync & Close", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
