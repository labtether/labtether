import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { BackupExportCard } from "../BackupExportCard";
import { fetchAllSchedules } from "../../../../../lib/schedules";

const mocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  downloadJSON: vi.fn(),
}));

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock("../../../../../lib/api", () => ({ apiFetch: mocks.apiFetch }));
vi.mock("../../../../../lib/export", () => ({ downloadJSON: mocks.downloadJSON }));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const schedules = Array.from({ length: 105 }, (_, index) => ({ id: `schedule-${index}` }));
const alertRules = Array.from({ length: 105 }, (_, index) => ({ id: `rule-${index}` }));
const channels = Array.from({ length: 105 }, (_, index) => ({ id: `channel-${index}` }));
const savedActions = Array.from({ length: 105 }, (_, index) => ({ id: `action-${index}` }));
let container: HTMLDivElement;
let root: Root;

function ok(data: unknown) {
  return { response: { ok: true, status: 200 }, data };
}

function setupFetch(
  pageTwo: "ok" | "failed" | "inconsistent" | "duplicate" = "ok",
  listFailure: "rules" | "channels" | "duplicate-rules" | null = null,
  actionFailure: "page" | "short" | "missing-meta" | "duplicate" | "huge-total" | null = null,
  payloadFailure: "assets" | "groups" | "webhooks" | null = null,
) {
  mocks.apiFetch.mockImplementation(async (url: string) => {
    if (url === "/api/v2/assets") {
      return ok(payloadFailure === "assets" ? {} : {
        data: [{ id: "asset-1" }], meta: { total: 1, page: 1, per_page: 1 },
      });
    }
    if (url === "/api/groups") {
      return ok(payloadFailure === "groups" ? {} : { groups: [{ id: "group-1" }] });
    }
    if (url === "/api/v2/webhooks") {
      return ok(payloadFailure === "webhooks" ? { webhooks: [] } : { data: [{ id: "webhook-1" }] });
    }
    if (url.startsWith("/api/v2/schedules?")) {
      const request = new URL(url, "http://localhost");
      const page = Number(request.searchParams.get("page"));
      if (page === 2 && pageTwo === "failed") {
        return { response: { ok: false, status: 503 }, data: null };
      }
      return ok({
        data: page === 1 ? schedules.slice(0, 100) : pageTwo === "duplicate"
          ? [schedules[99], ...schedules.slice(101)] : schedules.slice(100),
        meta: { total: page === 2 && pageTwo === "inconsistent" ? 104 : 105, page, per_page: 100 },
      });
    }
    if (url.startsWith("/api/v2/actions?")) {
      const offset = Number(new URL(url, "http://localhost").searchParams.get("offset"));
      if (offset === 100 && actionFailure === "page") {
        return { response: { ok: false, status: 503 }, data: null };
      }
      const page = savedActions.slice(offset, offset + 100);
      return ok({
        data: offset === 0 && actionFailure === "short" ? page.slice(0, 99)
          : offset === 100 && actionFailure === "duplicate" ? [savedActions[99], ...page.slice(1)]
            : page,
        meta: actionFailure === "missing-meta" ? undefined : {
          total: actionFailure === "huge-total" ? 10_001 : savedActions.length,
          page: offset / 100 + 1,
          per_page: 100,
        },
      });
    }
    if (url.startsWith("/api/alerts/rules?")) {
      const offset = Number(new URL(url, "http://localhost").searchParams.get("offset"));
      if (offset === 100 && listFailure === "rules") {
        return { response: { ok: false, status: 503 }, data: null };
      }
      const page = alertRules.slice(offset, offset + 100);
      return ok({ rules: offset === 100 && listFailure === "duplicate-rules"
        ? [alertRules[99], ...page.slice(1)] : page });
    }
    if (url.startsWith("/api/notifications/channels?")) {
      const offset = Number(new URL(url, "http://localhost").searchParams.get("offset"));
      if (offset === 100 && listFailure === "channels") {
        return { response: { ok: false, status: 503 }, data: null };
      }
      return ok({ channels: channels.slice(offset, offset + 100), capabilities: { smtp_insecure_transport_allowed: false } });
    }
    return ok([]);
  });
}

beforeEach(() => {
  mocks.apiFetch.mockReset();
  mocks.downloadJSON.mockReset();
  setupFetch();
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

async function exportConfiguration() {
  await act(async () => root.render(<BackupExportCard />));
  const button = container.querySelector("button");
  if (!button) throw new Error("Export button missing");
  await act(async () => button.click());
}

describe("configuration export schedule completeness", () => {
  it("exports every schedule when the API has more than one page", async () => {
    await exportConfiguration();

    expect(mocks.apiFetch).toHaveBeenCalledWith("/api/v2/schedules?page=1&per_page=100");
    expect(mocks.apiFetch).toHaveBeenCalledWith("/api/v2/schedules?page=2&per_page=100");
    expect(mocks.downloadJSON).toHaveBeenCalledTimes(1);
    const config = mocks.downloadJSON.mock.calls[0][0] as {
      schedules: { data: Array<{ id: string }>; meta: { total: number; page: number; per_page: number } };
      alert_rules: { rules: Array<{ id: string }> };
      notification_channels: { channels: Array<{ id: string }>; capabilities: { smtp_insecure_transport_allowed: boolean } };
      actions: Array<{ id: string }>;
      assets: { data: Array<{ id: string }> };
      groups: { groups: Array<{ id: string }> };
      webhooks: { data: Array<{ id: string }> };
    };
    expect(config.schedules.data.map((schedule) => schedule.id)).toEqual(schedules.map((schedule) => schedule.id));
    expect(config.schedules.meta).toEqual({ total: 105, page: 1, per_page: 105 });
    expect(config.alert_rules.rules.map((rule) => rule.id)).toEqual(alertRules.map((rule) => rule.id));
    expect(config.notification_channels.channels.map((channel) => channel.id)).toEqual(channels.map((channel) => channel.id));
    expect(config.notification_channels.capabilities).toEqual({ smtp_insecure_transport_allowed: false });
    expect(config.actions.map((action) => action.id)).toEqual(savedActions.map((action) => action.id));
    expect(config.assets.data).toEqual([{ id: "asset-1" }]);
    expect(config.groups.groups).toEqual([{ id: "group-1" }]);
    expect(config.webhooks.data).toEqual([{ id: "webhook-1" }]);
  });

  it("does not download or report success when the second page fails", async () => {
    setupFetch("failed");
    await exportConfiguration();

    expect(mocks.downloadJSON).not.toHaveBeenCalled();
    expect(container.textContent).toContain("Failed to load schedules (page 2, HTTP 503).");
    expect(container.textContent).not.toContain("backup.exportSuccess");
  });

  it("rejects inconsistent totals without fetching more pages", async () => {
    setupFetch("inconsistent");
    await expect(fetchAllSchedules()).rejects.toThrow("Incomplete schedules response (page 2).");
    expect(mocks.apiFetch).toHaveBeenCalledTimes(2);
  });

  it("rejects duplicate rows caused by a page boundary shift", async () => {
    setupFetch("duplicate");
    await expect(fetchAllSchedules()).rejects.toThrow("Incomplete schedules response (page 2).");
    expect(mocks.apiFetch).toHaveBeenCalledTimes(2);
  });

  it.each(["rules", "channels"] as const)("does not download when a later %s page fails", async (list) => {
    setupFetch("ok", list);
    await exportConfiguration();

    expect(mocks.downloadJSON).not.toHaveBeenCalled();
    expect(container.textContent).not.toContain("backup.exportSuccess");
    expect(container.textContent).toContain(`Failed to load /api/${list === "rules" ? "alerts/rules" : "notifications/channels"}`);
  });

  it("rejects duplicate alert rules across pages", async () => {
    setupFetch("ok", "duplicate-rules");
    await exportConfiguration();

    expect(mocks.downloadJSON).not.toHaveBeenCalled();
    expect(container.textContent).toContain("Incomplete /api/alerts/rules response (offset 100).");
  });

  it.each(["page", "short", "missing-meta", "duplicate", "huge-total"] as const)(
    "does not download when saved actions have a %s failure",
    async (failure) => {
      setupFetch("ok", null, failure);
      await exportConfiguration();

      expect(mocks.downloadJSON).not.toHaveBeenCalled();
      expect(container.textContent).not.toContain("backup.exportSuccess");
      expect(container.textContent).toContain(failure === "page"
        ? "Failed to load saved actions (offset 100, HTTP 503)."
        : "Incomplete saved actions response");
    },
  );

  it.each(["assets", "groups", "webhooks"] as const)(
    "does not download when the successful %s response is malformed",
    async (payload) => {
      setupFetch("ok", null, null, payload);
      await exportConfiguration();

      expect(mocks.downloadJSON).not.toHaveBeenCalled();
      expect(container.textContent).toContain("Incomplete inventory or webhook response.");
    },
  );
});
