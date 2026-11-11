import type { Asset } from "../../console/models";

/** Systems the Connect step adds through their API, as opposed to machines running the agent. */
const API_SOURCES = new Set(["proxmox", "pbs", "truenas", "docker", "portainer", "homeassistant", "home-assistant"]);

export type ChecklistStepId = "device" | "connect" | "terminal" | "notify" | "invite";

export type ChecklistStep = {
  id: ChecklistStepId;
  done: boolean;
  /** The name the done line mentions ("Agent on nas01"), or "". */
  sub: string;
  /** "add" opens Add device; otherwise the step links to href. */
  action?: "add";
  href?: string;
};

type Inputs = {
  assets: Pick<Asset, "id" | "name" | "source" | "status">[];
  recentCommandCount: number;
  hasChannels: boolean;
  users: Array<{ username: string; role?: string; created_at?: string }>;
  canWrite: boolean;
  canAdmin: boolean;
};

const up = (status: string) => status === "online" || status === "warning";
const nameOf = (a: { id: string; name: string }) => a.name || a.id;

/** The first one of a kind, preferring a device that answers. */
function pick<T extends { status: string }>(list: T[]): T | undefined {
  return list.find((a) => up(a.status)) ?? list[0];
}

/**
 * The first-run steps this person can take, each ticked from what the hub reports. Steps a role cannot act on are
 * left out unless they are already done; notifications and people are for admins.
 */
export function checklistSteps(input: Inputs): ChecklistStep[] {
  const agent = pick(input.assets.filter((a) => a.source.toLowerCase() === "agent"));
  const source = pick(input.assets.filter((a) => API_SOURCES.has(a.source.toLowerCase())));
  // The person the owner added: newest first. The Hub's user list has no creation time and is oldest first, so
  // without one the last person who is not the owner is the newest.
  const people = input.users.filter((u) => u.role !== "owner");
  const others = people.some((u) => u.created_at)
    ? [...people].sort((a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? ""))
    : [...people].reverse();
  const steps: ChecklistStep[] = [
    { id: "device", done: Boolean(agent), sub: agent ? nameOf(agent) : "", action: "add" },
    // Done when the Hub lists a system added through its API. The status call's connector list names the kinds
    // of connector the Hub supports, not ones that were set up, so it cannot tick this step.
    { id: "connect", done: Boolean(source), sub: source ? nameOf(source) : "", action: "add" },
    { id: "terminal", done: input.recentCommandCount > 0, sub: "", href: "/terminal" },
  ];
  const visible = steps.filter((s) => s.done || input.canWrite);
  if (input.canAdmin) {
    visible.push({ id: "notify", done: input.hasChannels, sub: "", href: "/settings/notifications" });
    visible.push({ id: "invite", done: input.users.length > 1, sub: input.users.length > 1 ? others[0]?.username ?? "" : "", href: "/users" });
  }
  return visible;
}
