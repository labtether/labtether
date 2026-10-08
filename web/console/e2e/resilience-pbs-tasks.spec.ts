import { expect,test } from "@playwright/test";
import {
buildLiveStatusPayload,
buildStatusPayload,
installConsoleApiMocks,
} from "./helpers/consoleApiMocks";
import { BASE_TS,makePBSAsset } from './helpers/resilienceMocks';

test("pbs task lifecycle works in node detail (status, log, stop)", async ({ page }) => {
  const pbsAsset = makePBSAsset();
  const taskUPID = "UPID:localhost:00000001:00000002:backup:vm/100:root@pam:";
  let stopCalls = 0;

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [pbsAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [pbsAsset],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/pbs/assets/${encodeURIComponent(pbsAsset.id)}/details`) {
        await fulfillJSON({
          asset_id: pbsAsset.id,
          kind: "server",
          collector_id: "collector-pbs-1",
          node: "localhost",
          version: "3.2.5",
          datastores: [
            {
              store: "backup",
              status: "ok",
              usage_percent: 55.2,
              used_bytes: 552,
              total_bytes: 1000,
              group_count: 4,
              snapshot_count: 22,
              last_backup_at: BASE_TS,
            },
          ],
          tasks: [
            {
              upid: taskUPID,
              node: "localhost",
              worker_type: "backup",
              worker_id: "vm/100",
              status: "running",
              starttime: 1_704_110_400,
            },
          ],
          warnings: [],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/status")) {
        await fulfillJSON({
          task: {
            upid: taskUPID,
            node: "localhost",
            status: "running",
          },
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/log")) {
        await fulfillJSON({
          lines: [
            { n: 1, t: "start backup job" },
            { n: 2, t: "backup complete" },
          ],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/stop") && method === "POST") {
        stopCalls++;
        await fulfillJSON({ ok: true });
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(pbsAsset.id)}?panel=pbs`);
  await expect(page.locator("main").getByRole("heading", { name: "Lab PBS", exact: true }).first()).toBeVisible();
  await page.getByRole("button", { name: "Tasks", exact: true }).click();
  await expect(page.getByText("Recent PBS Tasks")).toBeVisible();

  await page.getByRole("button").filter({ hasText: taskUPID }).click();
  await expect(page.getByText("Task Details")).toBeVisible();
  await expect(page.getByText("start backup job")).toBeVisible();

  await page.getByRole("button", { name: "Stop Task", exact: true }).click();
  await page.getByRole("button", { name: "Confirm Stop", exact: true }).click();
  await expect.poll(() => stopCalls).toBeGreaterThanOrEqual(1);
});

test("pbs task lifecycle surfaces status/log backend errors in node detail", async ({ page }) => {
  const pbsAsset = makePBSAsset();
  const taskUPID = "UPID:localhost:00000001:00000002:backup:vm/100:root@pam:";

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [pbsAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [pbsAsset],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/pbs/assets/${encodeURIComponent(pbsAsset.id)}/details`) {
        await fulfillJSON({
          asset_id: pbsAsset.id,
          kind: "server",
          collector_id: "collector-pbs-1",
          node: "localhost",
          tasks: [
            {
              upid: taskUPID,
              node: "localhost",
              worker_type: "backup",
              worker_id: "vm/100",
              status: "running",
              starttime: 1_704_110_400,
            },
          ],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/status")) {
        await fulfillJSON({ error: "status failed" }, 500);
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/log")) {
        await fulfillJSON({ error: "log failed" }, 500);
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(pbsAsset.id)}?panel=pbs`);
  await expect(page.locator("main").getByRole("heading", { name: "Lab PBS", exact: true }).first()).toBeVisible();
  await page.getByRole("button", { name: "Tasks", exact: true }).click();
  await expect(page.getByText("Recent PBS Tasks")).toBeVisible();

  const backupTaskButton = page.getByRole("button").filter({ hasText: taskUPID });
  await expect(backupTaskButton).toBeVisible();
  await backupTaskButton.click();
  await expect(page.getByText("Task Details")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("status failed")).toBeVisible({ timeout: 10_000 });
});

test("pbs task lifecycle handles stop backend 500 without breaking task panel", async ({ page }) => {
  const pbsAsset = makePBSAsset();
  const taskUPID = "UPID:localhost:00000001:00000002:backup:vm/100:root@pam:";
  const stopStatuses: number[] = [];

  page.on("response", (response) => {
    if (response.url().includes("/api/pbs/tasks/") && response.url().includes("/stop")) {
      stopStatuses.push(response.status());
    }
  });

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [pbsAsset],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [pbsAsset],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === `/api/pbs/assets/${encodeURIComponent(pbsAsset.id)}/details`) {
        await fulfillJSON({
          asset_id: pbsAsset.id,
          kind: "server",
          collector_id: "collector-pbs-1",
          node: "localhost",
          tasks: [
            {
              upid: taskUPID,
              node: "localhost",
              worker_type: "backup",
              worker_id: "vm/100",
              status: "running",
              starttime: 1_704_110_400,
            },
          ],
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/status")) {
        await fulfillJSON({
          task: {
            upid: taskUPID,
            node: "localhost",
            status: "running",
          },
        });
        return true;
      }
      if (pathname.startsWith("/api/pbs/tasks/localhost/") && pathname.endsWith("/log")) {
        await fulfillJSON({
          lines: [{ n: 1, t: "task still running" }],
        });
        return true;
      }
      if (pathname.includes("/api/pbs/tasks/") && pathname.endsWith("/stop")) {
        await fulfillJSON({ error: "stop failed" }, 500);
        return true;
      }
      return false;
    },
  });

  await page.goto(`/nodes/${encodeURIComponent(pbsAsset.id)}?panel=pbs`);
  await expect(page.locator("main").getByRole("heading", { name: "Lab PBS", exact: true }).first()).toBeVisible();
  await page.getByRole("button", { name: "Tasks", exact: true }).click();

  const backupTaskButton = page.getByRole("button").filter({ hasText: taskUPID });
  const taskDetailsHeading = page.getByText("Task Details");
  let taskDetailsVisible = false;
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await backupTaskButton.click();
    try {
      await expect(taskDetailsHeading).toBeVisible({ timeout: 2_500 });
      taskDetailsVisible = true;
      break;
    } catch {
      // Retry selection when list rerenders race with the first click.
    }
  }
  expect(taskDetailsVisible).toBe(true);
  await expect(page.getByText("task still running")).toBeVisible({ timeout: 10_000 });

  await page.getByRole("button", { name: "Stop Task", exact: true }).click();
  await page.getByRole("button", { name: "Confirm Stop", exact: true }).click();
  await page.waitForTimeout(750);
  if (stopStatuses.length > 0) {
    expect(stopStatuses.at(-1)).toBe(500);
  }
  await expect(page.getByRole("button", { name: "Stop Task", exact: true })).toBeEnabled();
  await expect(page.getByText("Task Details")).toBeVisible();
});
