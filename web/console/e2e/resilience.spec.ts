import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { BASE_TS } from './helpers/resilienceMocks';

test("expired deep-link session redirects to login and returns to target after sign-in", async ({ page }) => {
  let authenticated = false;
  const deepLink = "/nodes/node-1";

  const statusPayload = buildStatusPayload({
    assets: [
      {
        id: "node-1",
        type: "host",
        name: "node-1-host",
        source: "agent",
        status: "online",
        last_seen_at: BASE_TS,
      },
    ],
  });
  const liveStatusPayload = buildLiveStatusPayload({
    assets: statusPayload["assets"] as unknown[],
  });

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload,
    customRoute: async ({ pathname, method, requestBody, fulfillJSON }) => {
      if (pathname === "/api/auth/me") {
        if (!authenticated) {
          await fulfillJSON({ error: "unauthorized" }, 401);
        } else {
          await fulfillJSON({ user: { id: "owner", username: "admin", role: "owner" } }, 200);
        }
        return true;
      }
      if (pathname === "/api/auth/login" && method === "POST") {
        const username = String(requestBody.username ?? "");
        const password = String(requestBody.password ?? "");
        if (username === "admin" && password === "password") {
          authenticated = true;
          await fulfillJSON({ ok: true, user: { id: "owner", username: "admin", role: "owner" } }, 200);
        } else {
          await fulfillJSON({ error: "invalid credentials" }, 401);
        }
        return true;
      }
      return false;
    },
  });

  await page.goto(deepLink);
  await expect(page).toHaveURL(/\/login\?next=/);

  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();

  await expect(page).toHaveURL(/\/nodes\/node-1$/);
  await expect(
    page.locator("main").getByRole("heading", { name: "node-1-host", exact: true }).first(),
  ).toBeVisible();
});

test("login visibly announces rejected credentials", async ({ page }) => {
  await installConsoleApiMocks(page, {
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/auth/login" && method === "POST") {
        await fulfillJSON({ error: "invalid credentials" }, 401);
        return true;
      }
      return false;
    },
  });

  await page.goto("/login");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("wrong-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();

  await expect(page.getByRole("alert").filter({ hasText: "invalid credentials" })).toBeVisible();
  await expect(page.getByLabel("Username")).toHaveValue("admin");
  await expect(page.getByLabel("Password")).toHaveValue("");
});

test("login rejects unsafe next redirects and lands on root after sign-in", async ({ page }) => {
  await installConsoleApiMocks(page);

  await page.goto("/login?next=https%3A%2F%2Fevil.example%2Fsteal");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();

  // next-intl canonicalizes the default locale root to /en in production.
  // Either canonical form is local; the attacker-controlled origin must never
  // survive the redirect.
  await expect(page).toHaveURL(/\/(?:en)?$/);
  expect(new URL(page.url()).origin).toBe(test.info().project.use.baseURL
    ? new URL(String(test.info().project.use.baseURL)).origin
    : new URL(page.url()).origin);
});

test("origin guard rejects cross-origin mutating requests", async ({ request }) => {
  const sessionCookie = "labtether_session=test-session";
  const crossOriginHeaders = {
    origin: "https://evil.example",
    "sec-fetch-site": "cross-site",
    cookie: sessionCookie,
  };

  const calls: Array<{
    method: "POST" | "PATCH" | "DELETE";
    path: string;
    data?: Record<string, unknown>;
  }> = [
    {
      method: "POST",
      path: "/api/auth/login",
      data: { username: "admin", password: "password" },
    },
    {
      method: "POST",
      path: "/api/services/web/sync",
      data: {},
    },
    {
      method: "PATCH",
      path: "/api/settings/runtime",
      data: { values: {} },
    },
    {
      method: "POST",
      path: "/api/services/web/overrides",
      data: { host_asset_id: "host-1", service_id: "svc-1", hidden: false },
    },
    {
      method: "DELETE",
      path: "/api/services/web/overrides?host=host-1&service_id=svc-1",
    },
  ];

  for (const call of calls) {
    const response = await request.fetch(call.path, {
      method: call.method,
      headers: crossOriginHeaders,
      data: call.data,
    });
    expect(response.status(), `${call.method} ${call.path}`).toBe(403);

    const payload = await response.json();
    expect(payload).toMatchObject({ error: "forbidden origin" });
  }
});

test("origin guard allows same-origin mutating requests to reach route handlers", async ({ request }) => {
  const sessionCookie = "labtether_session=test-session";
  const sameOrigin = process.env.PLAYWRIGHT_BASE_URL
    ?? (process.env.PLAYWRIGHT_SELF_SIGNED_HTTPS === "1"
      ? "https://127.0.0.1:4173"
      : "http://127.0.0.1:4173");
  const sameOriginHeaders = {
    origin: sameOrigin,
    "sec-fetch-site": "same-origin",
    cookie: sessionCookie,
  };

  const calls: Array<{
    method: "POST" | "PATCH" | "DELETE";
    path: string;
    data?: Record<string, unknown>;
  }> = [
    {
      method: "POST",
      path: "/api/auth/login",
      data: { username: "admin", password: "password" },
    },
    {
      method: "POST",
      path: "/api/services/web/sync",
      data: {},
    },
    {
      method: "PATCH",
      path: "/api/settings/runtime",
      data: { values: {} },
    },
    {
      method: "POST",
      path: "/api/services/web/overrides",
      data: { host_asset_id: "host-1", service_id: "svc-1", hidden: false },
    },
    {
      method: "DELETE",
      path: "/api/services/web/overrides?host=host-1&service_id=svc-1",
    },
    {
      method: "POST",
      path: "/api/admin/reset",
      data: { confirm: false },
    },
  ];

  for (const call of calls) {
    const response = await request.fetch(call.path, {
      method: call.method,
      headers: sameOriginHeaders,
      data: call.data,
    });
    expect(response.status(), `${call.method} ${call.path}`).not.toBe(403);
  }
});
