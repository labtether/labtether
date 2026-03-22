import { expect, test } from "@playwright/test";
import { installConsoleApiMocks } from "./helpers/consoleApiMocks";

const VIEWPORTS = [
  { label: "mobile", width: 390, height: 844 },
  { label: "tablet", width: 768, height: 1024 },
  { label: "desktop", width: 1280, height: 800 },
] as const;

const ROUTES = [
  { path: "/nodes", heading: "Devices" },
  { path: "/logs", heading: "Logs" },
  { path: "/alerts", heading: "Alerts" },
  { path: "/settings", heading: "Settings" },
  { path: "/reliability", heading: "Health" },
] as const;

for (const viewport of VIEWPORTS) {
  test(`core console layouts fit ${viewport.label} viewport`, async ({ page }) => {
    await installConsoleApiMocks(page);
    await page.setViewportSize({ width: viewport.width, height: viewport.height });

    for (const route of ROUTES) {
      await page.goto(route.path);
      await expect(page.getByRole("heading", { name: route.heading, level: 1, exact: true })).toBeVisible();

      const overflowPx = await page.evaluate(() => {
        const root = document.documentElement;
        return Math.max(0, root.scrollWidth - window.innerWidth);
      });
      const overflowOffenders = await page.evaluate(() => {
        return Array.from(document.querySelectorAll<HTMLElement>("*"))
          .map((el) => {
            const rect = el.getBoundingClientRect();
            return {
              tag: el.tagName.toLowerCase(),
              className: el.className,
              id: el.id,
              right: Math.round(rect.right),
              width: Math.round(rect.width),
              text: (el.textContent ?? "").trim().slice(0, 32),
            };
          })
          .filter((entry) => entry.width > 0 && entry.right > window.innerWidth + 4)
          .sort((a, b) => b.right - a.right)
          .slice(0, 8);
      });
      expect(
        overflowPx,
        `route ${route.path} overflow offenders: ${JSON.stringify(overflowOffenders)}`,
      ).toBeLessThanOrEqual(4);

      const mobileToggle = page.getByRole("button", { name: "Toggle navigation", exact: true });
      if (viewport.width < 768) {
        await expect(mobileToggle).toBeVisible();
      } else {
        await expect(mobileToggle).toBeHidden();
      }
    }
  });
}

for (const viewport of VIEWPORTS) {
  test(`add-device modal remains usable on ${viewport.label}`, async ({ page }) => {
    await installConsoleApiMocks(page);
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await page.goto("/nodes");
    await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Add Device", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();

    const bounds = await dialog.boundingBox();
    expect(bounds).not.toBeNull();
    if (bounds) {
      expect(bounds.width).toBeLessThanOrEqual(viewport.width);
      expect(bounds.height).toBeLessThanOrEqual(viewport.height);
      expect(bounds.x).toBeGreaterThanOrEqual(0);
      expect(bounds.y).toBeGreaterThanOrEqual(0);
    }
  });
}
