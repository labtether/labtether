import { expect, test } from "@playwright/test";
import { installDesktopApiMocks } from "./helpers/desktopApiMocks";
import { installDesktopBrowserMocks } from "./helpers/desktopBrowserMocks";

test("desktop VNC credentials prompt supports retry after auth failure", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["username", "password"],
        outcome: "securityfailure",
        reason: "Authentication failed",
      },
      {
        types: ["username", "password"],
        outcome: "success",
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect(page.getByText("Authentication Required", { exact: true })).toBeVisible();
  await page.getByPlaceholder("Username").fill("operator");
  await page.getByPlaceholder("Password").fill("wrong-password");
  await page.getByRole("button", { name: "Authenticate" }).click();

  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Authentication failed", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          username: "operator",
          password: "wrong-password",
        }),
      },
    ]);

  await page.getByRole("button", { name: "Retry" }).click({ force: true });
  await expect(page.getByText("Authentication Required", { exact: true })).toBeVisible();
  await page.getByPlaceholder("Username").fill("operator");
  await page.getByPlaceholder("Password").fill("correct-password");
  await page.getByRole("button", { name: "Authenticate" }).click();

  await expect.poll(() => sessionRequests.length).toBe(2);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
});

test("desktop VNC password-only challenge auto-submits the session password", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["password"],
        outcome: "success",
      },
    ],
  });
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncPassword: "session-secret",
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect.poll(() => sessionRequests.length).toBe(1);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("VNC Password Required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
    ]);
});

test("desktop VNC password-only auto-submit resets across reconnects", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["password"],
        outcome: "success",
      },
      {
        types: ["password"],
        outcome: "success",
      },
    ],
  });
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncPassword: "session-secret",
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect.poll(() => sessionRequests.length).toBe(1);
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page.getByTitle("Disconnect").click();
  await expect(page.getByRole("button", { name: "Connect", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect.poll(() => sessionRequests.length).toBe(2);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("VNC Password Required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
    ]);
});

test("desktop VNC stored password falls back to the visible prompt after one failed auto-submit", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["password"],
        outcome: "repeatprompt",
      },
    ],
  });
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncPassword: "session-secret",
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("main select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect.poll(() => sessionRequests.length).toBe(1);
  await expect(page.getByText("VNC Password Required", { exact: true })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
    ]);

  await page.getByPlaceholder("Password").fill("manual-secret");
  await page.getByRole("button", { name: "Authenticate" }).click();

  await expect(page.getByText("VNC Password Required", { exact: true })).toHaveCount(0);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "manual-secret",
        }),
      },
    ]);
});
