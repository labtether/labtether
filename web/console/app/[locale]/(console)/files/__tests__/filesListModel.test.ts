import { describe, expect, it } from "vitest";
import { clickSelection, connectionAddress, formatModified, menuActions } from "../filesListModel";

describe("formatModified", () => {
  const now = new Date(2026, 9, 27, 22, 41);
  it("shows the clock today and the date otherwise", () => {
    expect(formatModified(new Date(2026, 9, 27, 2, 14).toISOString(), "en-GB", now)).toBe("02:14");
    expect(formatModified(new Date(2026, 9, 2, 9, 0).toISOString(), "en-GB", now)).toBe("2 Oct");
    expect(formatModified(new Date(2024, 11, 30).toISOString(), "en-GB", now)).toBe("30 Dec 2024");
    expect(formatModified("", "en-GB", now)).toBe("");
  });
});

// Picking rows decides what Delete… removes, so a click must never keep stale picks.
describe("clickSelection", () => {
  const order = ["a", "b", "c", "d"];
  it("picks one, toggles with the modifier and extends a run with Shift", () => {
    const one = clickSelection(new Set(["a", "c"]), order, "b", "a", { toggle: false, range: false });
    expect([...one.selected]).toEqual(["b"]);
    const toggled = clickSelection(one.selected, order, "b", one.anchor, { toggle: true, range: false });
    expect([...toggled.selected]).toEqual([]);
    const run = clickSelection(new Set(["a"]), order, "d", "b", { toggle: false, range: true });
    expect([...run.selected]).toEqual(["b", "c", "d"]);
    expect(run.anchor).toBe("b");
  });
});

// The menu must not offer changes on a read-only device tab, and must not offer Paste with nothing to paste.
describe("menuActions", () => {
  const file = { name: "notes.md", is_dir: false };
  it("offers changes only when writing is allowed", () => {
    expect(menuActions({ entry: file, count: 1, hasFiles: true, canPaste: true, canWrite: false }))
      .toEqual(["open", "download", "sep", "copy", "cut", "copyPath"]);
    expect(menuActions({ entry: file, count: 1, hasFiles: true, canPaste: false, canWrite: true }))
      .toEqual(["open", "download", "sep", "rename", "copy", "cut", "copyPath", "sep", "delete"]);
  });

  it("acts on several picks and on the folder background", () => {
    expect(menuActions({ entry: { name: "docs", is_dir: true }, count: 3, hasFiles: false, canPaste: true, canWrite: true }))
      .toEqual(["copy", "cut", "paste", "sep", "delete"]);
    expect(menuActions({ entry: null, count: 0, hasFiles: false, canPaste: true, canWrite: true }))
      .toEqual(["newFolder", "uploadHere", "paste", "sep", "refresh"]);
    expect(menuActions({ entry: null, count: 0, hasFiles: false, canPaste: false, canWrite: false })).toEqual(["refresh"]);
  });
});

// The row must still show a port someone set by hand; only the protocol's default is left out.
describe("connectionAddress", () => {
  it("drops default ports and shows the share or start folder", () => {
    expect(connectionAddress({ protocol: "smb", host: "10.20.0.21", port: 445, initial_path: "/", extra_config: { smb_share: "media" } })).toBe("10.20.0.21/media");
    expect(connectionAddress({ protocol: "sftp", host: "backup.example.net", port: 2222, initial_path: "/home/demo" })).toBe("backup.example.net:2222/home/demo");
  });
});
