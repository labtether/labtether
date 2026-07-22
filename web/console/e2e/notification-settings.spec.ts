import { expect, test } from "@playwright/test";

import { installConsoleApiMocks } from "./helpers/consoleApiMocks";

test("notification channels complete add, edit, test, toggle, and delete flow", async ({ page }) => {
  type Channel = {
    id: string;
    name: string;
    type: string;
    config: Record<string, unknown>;
    enabled: boolean;
    created_at: string;
    updated_at: string;
  };
  const channels: Channel[] = [];
  let testCalls = 0;

  await installConsoleApiMocks(page, {
    customRoute: async ({ method, pathname, requestBody, fulfillJSON, route }) => {
      if (pathname === "/api/notifications/channels" && method === "GET") {
        await fulfillJSON({
          channels,
          capabilities: { smtp_insecure_transport_allowed: false },
        });
        return true;
      }
      if (pathname === "/api/notifications/channels" && method === "POST") {
        expect(requestBody).toEqual({
          name: "Homelab Gotify",
          type: "gotify",
          enabled: true,
          config: {
            server_url: "https://gotify.example.test",
            app_token: "synthetic-app-token",
            priority: "5",
          },
        });
        channels.push({
          id: "channel-gotify-1",
          name: "Homelab Gotify",
          type: "gotify",
          config: { server_url: "https://gotify.example.test", priority: "5" },
          enabled: true,
          created_at: "2026-07-15T00:00:00Z",
          updated_at: "2026-07-15T00:00:00Z",
        });
        await fulfillJSON({ channel: channels[0] }, 201);
        return true;
      }
      if (pathname === "/api/notifications/channels/channel-gotify-1/test" && method === "POST") {
        testCalls += 1;
        if (testCalls === 1) {
          await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
        } else {
          await fulfillJSON({ success: true });
        }
        return true;
      }
      if (pathname === "/api/notifications/channels/channel-gotify-1" && method === "PATCH") {
        const channel = channels[0];
        if (!channel) throw new Error("missing mocked channel");
        if (typeof requestBody.enabled === "boolean") channel.enabled = requestBody.enabled;
        if (typeof requestBody.name === "string") channel.name = requestBody.name;
        await fulfillJSON({ channel });
        return true;
      }
      if (pathname === "/api/notifications/channels/channel-gotify-1" && method === "DELETE") {
        channels.splice(0, channels.length);
        await fulfillJSON({ status: "deleted" });
        return true;
      }
      return false;
    },
  });

  await page.goto("/settings", { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "+ Add Channel" }).click();
  const addDialog = page.getByRole("dialog", { name: "Select a channel type" });
  await expect(addDialog).toHaveAttribute("aria-modal", "true");
  await addDialog.getByRole("button", { name: /Gotify/ }).click();
  const addConfigDialog = page.getByRole("dialog", { name: "Gotify" });
  await addConfigDialog.getByLabel("Channel Name").fill("Homelab Gotify");
  await addConfigDialog.getByLabel("Server URL").fill("https://gotify.example.test");
  const appToken = addConfigDialog.getByLabel("App Token");
  await expect(appToken).toHaveAttribute("type", "password");
  await appToken.fill("synthetic-app-token");
  await addConfigDialog.getByLabel("Priority (optional)").fill("5");
  await addConfigDialog.getByRole("button", { name: "Save" }).click();

  await expect(page.getByText("Homelab Gotify", { exact: true })).toBeVisible();
  const enabledSwitch = page.getByRole("switch", { name: "Homelab Gotify: Enabled" });
  await enabledSwitch.click();
  await expect(page.getByRole("switch", { name: "Homelab Gotify: Disabled" })).toBeVisible();

  await page.getByRole("button", { name: "Channel actions" }).click();
  await page.getByRole("menuitem", { name: "Send Test" }).click();
  await expect(page.getByRole("status")).toContainText("Test failed: test delivery was not confirmed");

  await page.getByRole("button", { name: "Channel actions" }).click();
  await page.getByRole("menuitem", { name: "Send Test" }).click();
  await expect(page.getByRole("status")).toHaveText("Test sent");

  await page.getByRole("button", { name: "Channel actions" }).click();
  await page.getByRole("menuitem", { name: "Edit" }).click();
  const editDialog = page.getByRole("dialog", { name: "Gotify" });
  await editDialog.getByLabel("Channel Name").fill("Primary Gotify");
  await editDialog.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Primary Gotify", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Channel actions" }).click();
  await page.getByRole("menuitem", { name: "Delete" }).click();
  const deleteDialog = page.getByRole("dialog", { name: "Delete Channel" });
  await expect(deleteDialog).toContainText("Primary Gotify");
  await deleteDialog.getByRole("button", { name: "Delete Channel" }).click();
  await expect(page.getByText("No notification channels configured.", { exact: false })).toBeVisible();
  expect(testCalls).toBe(2);
});
