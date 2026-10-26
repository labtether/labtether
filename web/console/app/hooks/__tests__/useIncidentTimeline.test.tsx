import { act, type AnchorHTMLAttributes } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useIncidentDetail } from "../useIncidents";
import { IncidentCockpitCard } from "../../[locale]/(console)/incidents/IncidentCockpitCard";
import IncidentsPage from "../../[locale]/(console)/incidents/page";
import messages from "../../../messages/en/incidents.json";

// The incident page owns selection; Next's app-router Link is outside this fixture.
vi.mock("../../../i18n/navigation", () => ({
  Link: ({ href, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props} />
  ),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type PendingPage = { url: URL; signal?: AbortSignal; resolve: (response: Response) => void };
let pending: PendingPage[];
let container: HTMLDivElement;
let root: Root;
let fetchMock: ReturnType<typeof vi.fn>;

function Consumer({ id }: { id: string }) {
  const { incident, timeline } = useIncidentDetail(id);
  return incident ? <IncidentCockpitCard incident={incident} timeline={timeline} /> : null;
}

async function show(id: string) {
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" timeZone="UTC" messages={{ incidents: messages }}>
      <Consumer id={id} />
    </NextIntlClientProvider>,
  ));
}

function event(id: string, incidentID = "incident-a") {
  return {
    id, incident_id: incidentID, event_type: "alert_fired", source_ref: `alert:${id}`,
    summary: `Stored event ${id}`, occurred_at: "2026-10-10T09:00:00Z", created_at: "2026-10-11T10:00:00Z",
    metadata: { matched_assets: ["asset-a"], count: 2 },
  };
}

function incident(id: string) {
  return { id, title: id, status: "open", severity: "high", source: "manual", created_at: "2026-10-11T10:00:00Z" };
}

async function reply(page: PendingPage, body: unknown, status = 200) {
  await act(async () => page.resolve(Response.json(body, { status })));
}

function button(text: string): HTMLButtonElement {
  const found = [...container.querySelectorAll("button")].find((candidate) => candidate.textContent === text);
  expect(found, `Missing button: ${text}`).toBeDefined();
  return found!;
}

beforeEach(() => {
  pending = [];
  fetchMock = vi.fn((input: string, options?: RequestInit) => {
    const url = new URL(input, "https://console.example.test");
    if (url.pathname.endsWith("/events")) return new Promise<Response>((resolve) => {
      pending.push({ url, signal: options?.signal ?? undefined, resolve });
    });
    const id = decodeURIComponent(url.pathname.split("/").pop()!);
    return Promise.resolve(Response.json({ incident: incident(id) }));
  });
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

it("renders real stored fields, retries failed pages, and loads older history only on request", async () => {
  await show("incident-a");
  expect(pending).toHaveLength(1);
  expect(pending[0].url.searchParams.get("limit")).toBe("50");
  await reply(pending.shift()!, { error: "store unavailable" }, 500);
  expect(container.querySelector('[role="alert"]')?.textContent).toContain(messages.timeline.error);
  expect(container.textContent).not.toContain(messages.timeline.empty);

  await act(async () => button(messages.timeline.retry).click());
  await reply(pending.shift()!, { events: [{ ...event("newest"), created_at: "0001-01-01T00:00:00Z" }], next_cursor: "older/cursor" });
  expect(container.textContent).toContain("Stored event newest");
  expect(container.querySelector("time")?.dateTime).toBe("2026-10-10T09:00:00Z");
  expect(pending).toHaveLength(0); // No eager traversal of all retained history.

  await act(async () => { button(messages.timeline.loadOlder).click(); button(messages.timeline.loadOlder).click(); });
  expect(pending).toHaveLength(1);
  expect(pending[0].url.searchParams.get("cursor")).toBe("older/cursor");
  await reply(pending.shift()!, { error: "try again" }, 503);
  expect(container.textContent).toContain("Stored event newest");
  expect(container.querySelector('[role="alert"]')).not.toBeNull();

  await act(async () => button(messages.timeline.retry).click());
  expect(pending[0].url.searchParams.get("cursor")).toBe("older/cursor");
  await reply(pending.shift()!, { events: [event("newest"), event("older")], next_cursor: "" });
  expect(container.querySelectorAll("li")).toHaveLength(2);
  expect(container.textContent).toContain("Stored event older");
  expect(container.textContent).not.toContain(messages.timeline.loadOlder);
  expect(container.querySelector('[role="alert"]')).toBeNull();

  await act(async () => button(messages.timeline.refresh).click());
  expect(pending[0].url.searchParams.has("cursor")).toBe(false);
  await reply(pending.shift()!, { events: [] }); // Optional cursor supports the previous Hub envelope.
  expect(container.textContent).toContain(messages.timeline.empty);
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

it("aborts old incident pages and ignores late replies after selection changes", async () => {
  await show("incident-a");
  await reply(pending.shift()!, { events: [event("a")], next_cursor: "a-older" });
  await act(async () => button(messages.timeline.loadOlder).click());
  const stale = pending.shift()!;
  await show("incident-b");
  expect(stale.signal?.aborted).toBe(true);
  expect(container.textContent).not.toContain("Stored event a");
  expect(pending[0].url.searchParams.has("cursor")).toBe(false);
  await reply(pending.shift()!, { events: [event("b", "incident-b")] });
  await reply(stale, { events: [event("stale-a")], next_cursor: "more-a" });
  expect(container.querySelectorAll("li")).toHaveLength(1);
  expect(container.textContent).toContain("Stored event b");
  expect(container.textContent).not.toContain("stale-a");
  expect(container.textContent).not.toContain(messages.timeline.loadOlder);
});

it("shows detail failure and retry on the actual page, and ignores a canceled detail from another selection", async () => {
  const details: PendingPage[] = [];
  fetchMock.mockImplementation((input: string, options?: RequestInit) => {
    const url = new URL(input, "https://console.example.test");
    if (url.pathname === "/api/incidents") return Promise.resolve(Response.json({ incidents: [incident("incident-a"), incident("incident-b")] }));
    return new Promise<Response>((resolve) => {
      (url.pathname.endsWith("/events") ? pending : details).push({ url, signal: options?.signal ?? undefined, resolve });
    });
  });
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" timeZone="UTC" messages={{ incidents: messages }}><IncidentsPage /></NextIntlClientProvider>,
  ));
  const select = async (id: string) => {
    const row = [...container.querySelectorAll("li")].find((item) => item.textContent?.includes(id));
    expect(row).toBeDefined();
    await act(async () => row!.click());
  };
  await select("incident-a");
  await reply(details.shift()!, { error: "unavailable" }, 503);
  await reply(pending.shift()!, { events: [] });
  expect(container.querySelector('[role="alert"]')?.textContent).toBe(messages.detail.error);
  expect(container.textContent).not.toContain(messages.timeline.empty);
  await act(async () => button(messages.detail.retry).click());
  await reply(details.shift()!, { incident: incident("incident-a") });
  expect(container.querySelector('[role="alert"]')).toBeNull();
  expect([...container.querySelectorAll("h2")].some((heading) => heading.textContent === "incident-a")).toBe(true);

  await act(async () => button(messages.back).click());
  await select("incident-b");
  const staleDetail = details.shift()!;
  const stalePage = pending.shift()!;
  await act(async () => button(messages.back).click());
  expect(staleDetail.signal?.aborted).toBe(true);
  await select("incident-a");
  await reply(details.shift()!, { incident: incident("incident-a") });
  await reply(pending.shift()!, { events: [event("current-a")] });
  await reply(staleDetail, { incident: incident("incident-b") });
  await reply(stalePage, { events: [event("late-b", "incident-b")] });
  expect([...container.querySelectorAll("h2")].some((heading) => heading.textContent === "incident-a")).toBe(true);
  expect(container.textContent).not.toContain("incident-b");
  expect(container.textContent).not.toContain("late-b");
  expect(container.textContent).toContain("Stored event current-a");
});
