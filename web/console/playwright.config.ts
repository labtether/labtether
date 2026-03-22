import { defineConfig, devices } from "@playwright/test";

const useSelfSignedHttps = process.env.PLAYWRIGHT_SELF_SIGNED_HTTPS === "1";
const defaultBaseURL = useSelfSignedHttps
  ? "https://127.0.0.1:4173"
  : "http://127.0.0.1:4173";
const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? defaultBaseURL;
const useExistingServer = process.env.PLAYWRIGHT_USE_EXISTING_SERVER === "1";
const enableBrowserMatrix = process.env.PLAYWRIGHT_BROWSER_MATRIX === "1";
const internalHTTPPort = process.env.PLAYWRIGHT_HTTP_PORT ?? (useSelfSignedHttps ? "4174" : "4173");
const publicPort = process.env.PLAYWRIGHT_PUBLIC_PORT ?? "4173";
const webServerCommand = process.env.PLAYWRIGHT_WEB_SERVER_COMMAND
  ?? [
    "npm run build",
    "mkdir -p .next/standalone/.next",
    "rm -rf .next/standalone/.next/static .next/standalone/public",
    "cp -R .next/static .next/standalone/.next/static",
    "if [ -d public ]; then cp -R public .next/standalone/public; fi",
    useSelfSignedHttps
      ? `PLAYWRIGHT_HTTP_PORT=${internalHTTPPort} PLAYWRIGHT_HTTPS_PORT=${publicPort} node scripts/start-playwright-https.mjs`
      : `HOSTNAME=127.0.0.1 PORT=${internalHTTPPort} node .next/standalone/server.js`,
  ].join(" && ");

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: !useExistingServer,
  workers: useExistingServer ? 1 : undefined,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    ignoreHTTPSErrors: useSelfSignedHttps || process.env.PLAYWRIGHT_IGNORE_HTTPS_ERRORS === "1",
    trace: "on-first-retry",
    timezoneId: "UTC",
    locale: "en-US",
  },
  projects: enableBrowserMatrix
    ? [
      {
        name: "chromium",
        use: { ...devices["Desktop Chrome"] },
      },
      {
        name: "firefox",
        use: { ...devices["Desktop Firefox"] },
      },
      {
        name: "webkit",
        use: { ...devices["Desktop Safari"] },
      },
    ]
    : [
      {
        name: "chromium",
        use: { ...devices["Desktop Chrome"] },
      },
    ],
  webServer: useExistingServer
    ? undefined
    : {
      // Use a production server command by default to avoid `.next/dev/lock`
      // collisions with long-running local `next dev` sessions.
      command: webServerCommand,
      url: baseURL,
      ignoreHTTPSErrors: useSelfSignedHttps || process.env.PLAYWRIGHT_IGNORE_HTTPS_ERRORS === "1",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000
    }
});
