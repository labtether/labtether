import { describe, expect, it } from "vitest";

import { diskRows, failureText, poolLayout } from "../truenasModel";

// Risks: a pool line that miscounts its disks or misnames its layout, a SMART-failing disk that drops out of the
// list or sinks below healthy ones, and a bare status code reaching the screen.

const disk = (name: string) => ({ name, type: "DISK", status: "ONLINE" });

describe("poolLayout", () => {
  it("names the data layout and counts the disks under every vdev", () => {
    expect(poolLayout([{ name: "raidz2-0", type: "RAIDZ2", status: "ONLINE", children: ["a", "b", "c", "d", "e", "f", "g", "h"].map(disk) }]))
      .toEqual({ kind: "RAIDZ2", groups: 1, disks: 8 });
    expect(poolLayout([
      { name: "mirror-0", type: "MIRROR", status: "ONLINE", children: [disk("a"), disk("b")] },
      { name: "mirror-1", type: "MIRROR", status: "ONLINE", children: [disk("c"), disk("d")] },
    ])).toEqual({ kind: "MIRROR", groups: 2, disks: 4 });
    expect(poolLayout([disk("/dev/sda"), disk("/dev/sdb")])).toEqual({ kind: "STRIPE", groups: 1, disks: 2 });
    expect(poolLayout(undefined)).toBeNull();
  });
});

describe("diskRows", () => {
  it("keeps disks known only to SMART and lists faults first", () => {
    const rows = diskRows(
      [{ name: "sda", model: "A" }, { name: "sdb", smart_status: "FAILED" }, { name: "sdc", smart_status: "PASSED" }],
      [{ name: "sdd", status: "critical", smart_health: "failing" }, { name: "sda", status: "healthy", last_test_at: "2026-10-11T02:00:00Z" }],
    );
    expect(rows.map((row) => row.name)).toEqual(["sdd", "sdb", "sda", "sdc"]);
    expect(rows[0].fault).toEqual({ tone: "bad", text: "failing" });
    expect(rows[1].fault).toEqual({ tone: "warn", text: "FAILED" });
    expect(rows[2].fault).toBeNull();
    expect(rows[2].lastTest?.at).toBe("2026-10-11T02:00:00Z");
  });
});

describe("failureText", () => {
  it("drops bare status codes and falls back when nothing else is said", () => {
    expect(failureText("action failed (500)", "TrueNAS did not say why")).toBe("TrueNAS did not say why");
    expect(failureText("pool tank is busy (409)", "x")).toBe("pool tank is busy");
    expect(failureText("", "fallback")).toBe("fallback");
  });
});
