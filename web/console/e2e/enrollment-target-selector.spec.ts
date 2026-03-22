import { expect, test } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./helpers/consoleApiMocks";

const DEFAULT_HUB_URL = "https://labtail.ts.net";
const DEFAULT_WS_URL = "wss://labtail.ts.net/ws/agent";
const LAN_HUB_URL = "http://192.168.1.25:8080";
const LAN_WS_URL = "ws://192.168.1.25:8080/ws/agent";

test("enrollment connection target persists and add-device installer command follows the selected hub", async ({ page }) => {
  let tokenCounter = 0;
  const statusPayload = buildStatusPayload();
  const liveStatusPayload = buildLiveStatusPayload();

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/settings/enrollment") {
        if (method === "POST") {
          tokenCounter += 1;
          await fulfillJSON({
            token: { id: `tok-${tokenCounter}` },
            raw_token: `enroll-token-${tokenCounter}`,
          }, 201);
          return true;
        }
        await fulfillJSON({
          tokens: [],
          hub_url: DEFAULT_HUB_URL,
          ws_url: DEFAULT_WS_URL,
          hub_candidates: [
            {
              kind: "tailscale",
              label: "Tailscale",
              host: "labtail.ts.net",
              hub_url: DEFAULT_HUB_URL,
              ws_url: DEFAULT_WS_URL,
            },
            {
              kind: "lan",
              label: "LAN",
              host: "192.168.1.25",
              hub_url: LAN_HUB_URL,
              ws_url: LAN_WS_URL,
            },
          ],
        });
        return true;
      }
      if (pathname === "/api/services/web/compat" && method === "GET") {
        await fulfillJSON({ compatible: [] });
        return true;
      }
      return false;
    },
  });

  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();

  const settingsTargetSelect = page.getByLabel("Connection target");
  await expect(settingsTargetSelect).toHaveValue(DEFAULT_HUB_URL);
  await settingsTargetSelect.selectOption(LAN_HUB_URL);

  await expect(settingsTargetSelect).toHaveValue(LAN_HUB_URL);
  await expect(page.getByText(LAN_HUB_URL, { exact: true })).toBeVisible();
  await expect(page.getByText(LAN_WS_URL, { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Create Token", exact: true }).click();
  await expect(page.getByText("Your enrollment token is ready — copy it now:", { exact: true })).toBeVisible();
  await page.getByText("Copy/paste setup", { exact: true }).click();
  await expect(page.locator("pre").filter({ hasText: LAN_WS_URL }).first()).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();
  await expect(page.getByLabel("Connection target")).toHaveValue(LAN_HUB_URL);

  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  await page.getByRole("button", { name: /^Agent/i }).first().click();

  await expect(page.getByText("Install Agent", { exact: true })).toBeVisible();
  await expect(page.locator("select").first()).toHaveValue(LAN_HUB_URL);
  await expect(page.getByText(LAN_HUB_URL, { exact: true })).toBeVisible();
  await expect(page.getByText(LAN_WS_URL, { exact: true })).toBeVisible();
  await expect(page.locator("pre").filter({ hasText: `${LAN_HUB_URL}/install.sh` }).first()).toBeVisible();
});
