import { expect, test, type Page } from "@playwright/test";
import { EXPLICIT_ID, GO_ID, MONITORING_ID, installAgentOffersMocks } from "./helpers/agentOffersFixtures";

// A device whose agent lists what it offers shows only that; every other device looks as it did before.

async function openDevice(page: Page, id: string, panel = "") {
  await page.goto(`/nodes/${id}${panel ? `?panel=${panel}` : ""}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: id, level: 1, exact: true })).toBeVisible();
}

async function menuItems(page: Page): Promise<string[]> {
  await page.getByRole("button", { name: "Device actions", exact: true }).click();
  const items = await page.getByRole("menuitem").allInnerTexts();
  await page.keyboard.press("Escape");
  return items.map((text) => text.trim());
}

test.beforeEach(async ({ page }) => {
  // The devices last checked in at 10:00; keep them online.
  await page.clock.setFixedTime(new Date("2026-11-16T10:00:30.000Z"));
  await installAgentOffersMocks(page);
});

test("the monitoring edition shows no Terminal, Files or power controls and is labelled", async ({ page }) => {
  await openDevice(page, MONITORING_ID);
  const head = page.locator(".dv-head");
  await expect(head).toContainText("Monitoring only");
  await expect(head.getByRole("link", { name: "Terminal", exact: true })).toHaveCount(0);
  await expect(head.getByRole("link", { name: "Files", exact: true })).toHaveCount(0);
  await expect(head.getByRole("link", { name: "Remote view", exact: true })).toHaveCount(0);
  await expect(page.locator(".dv-props")).toContainText("Monitoring only");

  const items = await menuItems(page);
  for (const gone of ["Terminal", "Files", "Remote view", "Restart…", "Shut down…"]) expect(items).not.toContain(gone);
  expect(items).toContain("Edit…");

  // The System tab offers the network and disk lists, not Processes, Services or Users.
  await page.locator(".dv-tabs").getByRole("link", { name: "System", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/nodes/${MONITORING_ID}\\?panel=network`));
  const views = page.getByRole("navigation", { name: "System view", exact: true });
  await expect(views.getByRole("link")).toHaveText(["Network", "Disks", "Hardware", "Metrics"]);
});

test("a panel opened by address for something the agent does not offer says so", async ({ page }) => {
  await openDevice(page, MONITORING_ID, "terminal");
  await expect(page.getByText("Not offered by this device's agent", { exact: true })).toBeVisible();
});

test("an explicit agent keeps what it offers and loses power", async ({ page }) => {
  await openDevice(page, EXPLICIT_ID);
  const head = page.locator(".dv-head");
  await expect(head.getByRole("link", { name: "Terminal", exact: true })).toBeVisible();
  await expect(head).not.toContainText("Monitoring only");
  const items = await menuItems(page);
  expect(items).not.toContain("Restart…");
  expect(items).not.toContain("Shut down…");
});

test("a Go agent device is unchanged", async ({ page }) => {
  await openDevice(page, GO_ID);
  const head = page.locator(".dv-head");
  await expect(head.getByRole("link", { name: "Terminal", exact: true })).toBeVisible();
  await expect(head.getByRole("link", { name: "Files", exact: true })).toBeVisible();
  await expect(head).not.toContainText("Monitoring only");
  const items = await menuItems(page);
  expect(items).toEqual(expect.arrayContaining(["Remote view", "Restart…", "Shut down…"]));
});

test("the Devices list labels the monitoring edition only", async ({ page }) => {
  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();
  const row = (id: string) => page.locator(".ld-row").filter({ has: page.getByRole("link", { name: id, exact: true }) });
  await expect(row(MONITORING_ID)).toContainText("macOS 15.4 · Monitoring only");
  await expect(row(GO_ID)).not.toContainText("Monitoring only");
});

test("the Terminal and Files pickers leave out a device whose agent does not offer them", async ({ page }) => {
  await page.goto("/terminal", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("searchbox", { name: "Connect to…" })).toBeVisible();
  const rows = page.locator("[data-tm-row]");
  await expect(rows.filter({ hasText: GO_ID }).first()).toBeVisible();
  await expect(rows.filter({ hasText: EXPLICIT_ID }).first()).toBeVisible();
  await expect(rows.filter({ hasText: MONITORING_ID })).toHaveCount(0);

  await page.goto("/files", { waitUntil: "domcontentloaded" });
  const ready = page.locator("button.fx-prow");
  await expect(ready.filter({ hasText: GO_ID })).toBeVisible();
  await expect(ready.filter({ hasText: MONITORING_ID })).toHaveCount(0);
});
