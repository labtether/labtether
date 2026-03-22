import { expect, test, type Page } from "@playwright/test";
import { installConsoleApiMocks } from "./helpers/consoleApiMocks";

const ROUTES = [
  { path: "/settings", heading: "Settings" },
  { path: "/alerts", heading: "Alerts" },
  { path: "/logs", heading: "Logs" },
  { path: "/reliability", heading: "Health" },
  { path: "/nodes", heading: "Devices" },
] as const;

const THEMES = ["oled", "dark", "light"] as const;

async function tabUntil(page: Page, predicate: () => Promise<boolean>, maxTabs = 40): Promise<boolean> {
  for (let i = 0; i < maxTabs; i++) {
    await page.keyboard.press("Tab");
    if (await predicate()) {
      return true;
    }
  }
  return false;
}

test("add device modal traps keyboard focus and closes with Escape", async ({ page, browserName }) => {
  await installConsoleApiMocks(page);
  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Add Device", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();

  if (browserName !== "webkit") {
    for (let i = 0; i < 18; i++) {
      await page.keyboard.press("Tab");
      const isInside = await page.evaluate(() => {
        const modal = document.querySelector("[role='dialog']");
        return Boolean(modal && document.activeElement && modal.contains(document.activeElement));
      });
      expect(isInside).toBeTruthy();
    }

    await page.keyboard.press("Shift+Tab");
    const stillInside = await page.evaluate(() => {
      const modal = document.querySelector("[role='dialog']");
      return Boolean(modal && document.activeElement && modal.contains(document.activeElement));
    });
    expect(stillInside).toBeTruthy();
  }

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("core page headings keep accessible contrast across themes", async ({ page }) => {
  await installConsoleApiMocks(page);

  for (const theme of THEMES) {
    await page.addInitScript((nextTheme) => {
      localStorage.setItem("labtether.theme", nextTheme);
      localStorage.setItem("labtether.density", "minimal");
    }, theme);

    for (const route of ROUTES) {
      await page.goto(route.path, { waitUntil: "domcontentloaded" });
      await page.waitForLoadState("domcontentloaded");
      await expect(page.locator("h1").first()).toBeVisible();

      const ratio = await page.evaluate(() => {
        function parseColor(input: string): [number, number, number] | null {
          const trimmed = input.trim().toLowerCase();
          const rgbMatch = trimmed.match(/^rgba?\(([^)]+)\)$/);
          if (rgbMatch) {
            const parts = rgbMatch[1].split(",").map((value) => Number(value.trim()));
            if (parts.length >= 3 && parts.slice(0, 3).every((value) => Number.isFinite(value))) {
              return [parts[0], parts[1], parts[2]];
            }
          }
          const hexMatch = trimmed.match(/^#([0-9a-f]{6})$/);
          if (hexMatch) {
            const value = hexMatch[1];
            return [
              Number.parseInt(value.slice(0, 2), 16),
              Number.parseInt(value.slice(2, 4), 16),
              Number.parseInt(value.slice(4, 6), 16),
            ];
          }
          return null;
        }

        function toLinear(channel: number): number {
          const normalized = channel / 255;
          return normalized <= 0.03928
            ? normalized / 12.92
            : ((normalized + 0.055) / 1.055) ** 2.4;
        }

        function luminance(rgb: [number, number, number]): number {
          return 0.2126 * toLinear(rgb[0]) + 0.7152 * toLinear(rgb[1]) + 0.0722 * toLinear(rgb[2]);
        }

        function ratio(fg: [number, number, number], bg: [number, number, number]): number {
          const lighter = Math.max(luminance(fg), luminance(bg));
          const darker = Math.min(luminance(fg), luminance(bg));
          return (lighter + 0.05) / (darker + 0.05);
        }

        const heading = document.querySelector("h1");
        if (!(heading instanceof HTMLElement)) return 0;

        const fg = parseColor(getComputedStyle(heading).color);
        if (!fg) return 0;

        let bgColor = "";
        let current: HTMLElement | null = heading;
        while (current) {
          bgColor = getComputedStyle(current).backgroundColor;
          if (bgColor && !bgColor.includes("rgba(0, 0, 0, 0)") && !bgColor.endsWith(", 0)")) {
            break;
          }
          current = current.parentElement;
        }
        if (!bgColor || bgColor.includes("rgba(0, 0, 0, 0)") || bgColor.endsWith(", 0)")) {
          bgColor = getComputedStyle(document.body).backgroundColor;
        }
        const bg = parseColor(bgColor);
        if (!bg) return 0;
        return ratio(fg, bg);
      });

      expect(ratio).toBeGreaterThanOrEqual(4.5);
    }
  }
});

test("keyboard navigation reaches sidebar links and add-device flow", async ({ page, browserName }) => {
  test.skip(
    browserName === "webkit",
    "WebKit link tab order depends on system full keyboard access settings.",
  );

  await installConsoleApiMocks(page);
  await page.goto("/nodes", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Devices", level: 1, exact: true })).toBeVisible();

  const reachedDashboardLink = await tabUntil(page, async () => {
    const pathname = await page.evaluate(() => {
      const href = (document.activeElement as HTMLAnchorElement | null)?.getAttribute("href");
      if (!href) return "";
      return new URL(href, window.location.origin).pathname;
    });
    return pathname === "/" || /^\/[a-z]{2}$/.test(pathname);
  });
  expect(reachedDashboardLink).toBeTruthy();

  const reachedAddDevice = await tabUntil(page, async () => {
    const name = await page.evaluate(() => {
      const active = document.activeElement as HTMLElement | null;
      return active?.textContent?.trim() ?? "";
    });
    return name.includes("Add Device");
  }, 60);
  expect(reachedAddDevice).toBeTruthy();

  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();

  const reachedAgentSource = await tabUntil(page, async () => {
    const active = await page.evaluate(() => {
      const element = document.activeElement as HTMLElement | null;
      if (!element) return "";
      const text = element.textContent?.trim() ?? "";
      return `${element.tagName}:${text}`;
    });
    return active.includes("BUTTON:Agent");
  }, 30);
  expect(reachedAgentSource).toBeTruthy();

  await page.keyboard.press("Enter");
  await expect(page.getByText("Install Agent")).toBeVisible();
});
