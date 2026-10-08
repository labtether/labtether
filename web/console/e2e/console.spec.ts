import { expect,test } from "@playwright/test";
import { mockConsoleBootstrap } from './helpers/consoleBootstrap';

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
