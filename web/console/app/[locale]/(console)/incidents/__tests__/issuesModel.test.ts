import { describe, expect, it } from "vitest";
import { filterIssues, nextStates, sortIssues, STATES, type Issue } from "../issuesModel";

const issue = (id: string, status: Issue["status"], opened: string, assignee = ""): Issue => ({
  id, title: id, status, severity: "high", source: "manual", assignee,
  created_at: opened, updated_at: opened, opened_at: opened,
});

describe("issue state menu", () => {
  // Mirrors incidents.CanTransitionStatus in the hub: any other move is refused on save.
  const HUB: Record<string, string[]> = {
    open: ["investigating", "resolved", "closed"],
    investigating: ["mitigated", "resolved", "closed"],
    mitigated: ["investigating", "resolved", "closed"],
    resolved: ["investigating", "closed"],
    closed: ["investigating"],
  };
  it("offers exactly the moves the hub accepts, never the current state", () => {
    for (const from of STATES) {
      expect([...nextStates(from)].sort()).toEqual([...HUB[from]].sort());
      expect(nextStates(from)).not.toContain(from);
    }
  });
});

describe("issue list", () => {
  const list = [
    issue("a", "open", "2026-10-30T10:00:00Z"),
    issue("b", "closed", "2026-10-31T10:00:00Z", "sam"),
    issue("c", "mitigated", "2026-10-29T10:00:00Z", "alex"),
    issue("d", "resolved", "2026-10-28T10:00:00Z"),
  ];

  it("counts closed issues as resolved and keeps an exact state filter exact", () => {
    expect(filterIssues(list, "active").map((i) => i.id)).toEqual(["a", "c"]);
    expect(filterIssues(list, "resolved").map((i) => i.id)).toEqual(["b", "d"]);
    expect(filterIssues(list, "closed").map((i) => i.id)).toEqual(["b"]);
    expect(filterIssues(list, "all")).toHaveLength(4);
  });

  it("sorts newest first by default and puts unassigned issues after every owner", () => {
    expect(sortIssues(list, { key: "opened", dir: "desc" }).map((i) => i.id)).toEqual(["b", "a", "c", "d"]);
    expect(sortIssues(list, { key: "owner", dir: "asc" }).map((i) => i.id)).toEqual(["c", "b", "a", "d"]);
    expect(sortIssues(list, { key: "state", dir: "asc" }).map((i) => i.id)).toEqual(["a", "c", "d", "b"]);
  });

  it("falls back to the creation time when the hub sends no usable opened time", () => {
    const older = { ...issue("old", "open", "2026-10-01T10:00:00Z"), opened_at: "0001-01-01T00:00:00Z" };
    expect(sortIssues([older, list[0]], { key: "opened", dir: "desc" }).map((i) => i.id)).toEqual(["a", "old"]);
  });
});
