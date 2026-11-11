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
          all_alerts: true,
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
          all_alerts: true,
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

  await page.goto("/settings/notifications", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add channel", exact: true }).click();
  const addDialog = page.getByRole("dialog", { name: "Add channel" });
  await expect(addDialog).toHaveAttribute("aria-modal", "true");
  await addDialog.getByRole("button", { name: /Gotify/ }).click();
  const addConfigDialog = page.getByRole("dialog", { name: "Gotify" });
  await addConfigDialog.getByLabel("Name", { exact: true }).fill("Homelab Gotify");
  await addConfigDialog.getByLabel("Server URL").fill("https://gotify.example.test");
  const appToken = addConfigDialog.getByLabel("App token");
  await expect(appToken).toHaveAttribute("type", "password");
  await appToken.fill("synthetic-app-token");
  await addConfigDialog.getByLabel("Priority (optional)").fill("5");
  await addConfigDialog.getByRole("button", { name: "Save" }).click();

  await expect(page.getByText("Homelab Gotify", { exact: true })).toBeVisible();
  const enabledSwitch = page.getByRole("switch", { name: "Send to Homelab Gotify" });
  await expect(enabledSwitch).toHaveAttribute("aria-checked", "true");
  await enabledSwitch.click();
  await expect(enabledSwitch).toHaveAttribute("aria-checked", "false");
  // A turned-off channel cannot send a test; turn it back on.
  await expect(page.getByRole("button", { name: "Send test" })).toBeDisabled();
  await enabledSwitch.click();
  await expect(enabledSwitch).toHaveAttribute("aria-checked", "true");

  await page.getByRole("button", { name: "Send test" }).click();
  await expect(page.getByRole("status")).toContainText("Test failed: test delivery was not confirmed");

  await page.getByRole("button", { name: "Send test" }).click();
  await expect(page.getByRole("status")).toHaveText("Test sent");

  await page.getByRole("button", { name: "Actions for Homelab Gotify" }).click();
  await page.getByRole("menuitem", { name: "Edit…" }).click();
  const editDialog = page.getByRole("dialog", { name: "Gotify" });
  await editDialog.getByLabel("Name", { exact: true }).fill("Primary Gotify");
  await editDialog.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Primary Gotify", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Actions for Primary Gotify" }).click();
  await page.getByRole("menuitem", { name: "Delete…" }).click();
  const deleteDialog = page.getByRole("dialog", { name: "Delete Primary Gotify?" });
  await deleteDialog.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page.getByText("No channels yet", { exact: true })).toBeVisible();
  expect(testCalls).toBe(2);
});
