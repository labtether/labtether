import { describe, expect, it } from "vitest";
import { checklistSteps } from "../firstRunSteps";

// Risks: the Dashboard's first-run checklist ticks a step the person has not done (a Proxmox node counted as "a
// device with the agent"), or offers a step their role cannot take.

const base = { assets: [], recentCommandCount: 0, hasChannels: false, users: [], canWrite: true, canAdmin: false };

describe("checklistSteps", () => {
  it("ticks the agent and connect steps from the right kind of device", () => {
    const steps = checklistSteps({
      ...base,
      assets: [
        { id: "pve-01", name: "pve-01", source: "proxmox", status: "online" },
        { id: "nas01", name: "nas01", source: "agent", status: "offline" },
        { id: "nuc-01", name: "nuc-01", source: "agent", status: "online" },
      ],
    });
    expect(steps.find((s) => s.id === "device")).toMatchObject({ done: true, sub: "nuc-01" });
    expect(steps.find((s) => s.id === "connect")).toMatchObject({ done: true, sub: "pve-01" });
    expect(steps.find((s) => s.id === "terminal")?.done).toBe(false);
  });

  it("leaves out steps a viewer cannot take and adds the admin ones only for admins", () => {
    expect(checklistSteps({ ...base, canWrite: false }).map((s) => s.id)).toEqual([]);
    expect(checklistSteps({ ...base, canWrite: false, recentCommandCount: 2 }).map((s) => s.id)).toEqual(["terminal"]);
    const admin = checklistSteps({ ...base, canAdmin: true, hasChannels: true, users: [
      { username: "owner", created_at: "2026-10-01T00:00:00Z" },
      { username: "sam", created_at: "2026-10-20T00:00:00Z" },
    ] });
    expect(admin.map((s) => s.id)).toEqual(["device", "connect", "terminal", "notify", "invite"]);
    expect(admin.find((s) => s.id === "notify")?.done).toBe(true);
    expect(admin.find((s) => s.id === "invite")).toMatchObject({ done: true, sub: "sam" });
  });

  it("names the person the owner added, not the owner, from the Hub's oldest-first user list", () => {
    // The Hub's /auth/users answer has no created_at and lists the owner first.
    const steps = checklistSteps({ ...base, canAdmin: true, users: [
      { username: "owner", role: "owner" },
      { username: "sam", role: "operator" },
      { username: "kim", role: "viewer" },
    ] });
    expect(steps.find((s) => s.id === "invite")).toMatchObject({ done: true, sub: "kim" });
  });
});
