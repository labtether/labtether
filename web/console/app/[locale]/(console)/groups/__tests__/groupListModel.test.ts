import { describe, expect, it } from "vitest";
import type { Group } from "../../../../console/models";
import { deviceNamesByGroup, firstNames, groupPath, slugFromName, ungroupedCount, visibleRows } from "../groupListModel";
import type { GroupTreeItem } from "../useGroupTree";

const group = (id: string, parent?: string): Group => ({ id, name: id.toUpperCase(), slug: id, parent_group_id: parent, sort_order: 0, created_at: "", updated_at: "" });
const item = (id: string, depth: number, expanded: boolean, children: GroupTreeItem[] = []): GroupTreeItem =>
  ({ id: `group:${id}`, group: group(id), depth, expanded, children, deviceCount: 0 });

// A folded group must hide its whole subtree; an open one shows children right under it.
describe("visibleRows", () => {
  it("shows open children in tree order and hides folded ones", () => {
    const tree = [
      item("home", 0, true, [item("rack", 1, false, [item("shelf", 2, true)]), item("desk", 1, true)]),
      item("lab", 0, false, [item("hidden", 1, true)]),
    ];
    expect(visibleRows(tree).map((row) => row.group.id)).toEqual(["home", "rack", "desk", "lab"]);
  });
});

// The "Not in a group" row must also count devices whose group the hub no longer lists.
describe("ungroupedCount", () => {
  it("counts devices without a group or with an unknown one", () => {
    const assets = [{ group_id: "home" }, { group_id: "" }, {}, { group_id: "gone" }];
    expect(ungroupedCount(assets, [group("home")])).toBe(3);
  });
});

describe("row grey line parts", () => {
  it("names a group's own devices in order and counts the rest", () => {
    const names = deviceNamesByGroup([
      { name: "nas01", group_id: "home" }, { name: "docker-01", group_id: "home" }, { name: "pve-01", group_id: " home " }, { name: "x", group_id: undefined },
    ]);
    expect(names.get("home")).toEqual(["docker-01", "nas01", "pve-01"]);
    expect(firstNames(["a", "b", "c", "d", "e", "f"])).toEqual({ shown: ["a", "b", "c", "d"], more: 2 });
  });

  it("builds a path from the top and a slug the hub accepts", () => {
    expect(groupPath([group("home"), group("rack", "home")], "rack")).toBe("HOME / RACK");
    expect(slugFromName("  Rack Ä / 2 ")).toBe("rack-a-2");
  });
});
