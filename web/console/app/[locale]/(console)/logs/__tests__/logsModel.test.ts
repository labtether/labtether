import { describe, expect, it } from "vitest";
import type { LogEvent } from "../../../../console/models";
import { contextLines, levelWord, logTime, newestFirst, selectedText } from "../logsModel";

// Risks: an open line shows another device's lines as its context, a copied selection comes out in the wrong order,
// or an agent's "warning"/"critical" spelling loses its colour.

const line = (id: string, asset: string, minute: number, level = "info"): LogEvent => ({
  id,
  asset_id: asset,
  source: "agent",
  level,
  message: `m-${id}`,
  timestamp: new Date(2026, 9, 27, 22, minute, 0).toISOString(),
});

describe("logsModel", () => {
  it("maps level spellings to the word the row shows", () => {
    expect(levelWord("WARNING")).toBe("warn");
    expect(levelWord("critical")).toBe("error");
    expect(levelWord("trace")).toBe("debug");
    expect(levelWord("notice")).toBe("info");
  });

  it("writes today's time with seconds and older days with a weekday or date", () => {
    const now = new Date(2026, 9, 27, 23, 0, 0);
    expect(logTime(new Date(2026, 9, 27, 22, 40, 12).toISOString(), now)).toBe("22:40:12");
    expect(logTime(new Date(2026, 9, 26, 9, 5, 0).toISOString(), now)).toBe("Mon 09:05");
    expect(logTime(new Date(2026, 9, 10, 9, 5, 0).toISOString(), now)).toMatch(/^10 Oct 09:05$|^Oct 10 09:05$/);
    expect(logTime("not a time", now)).toBe("not a time");
  });

  it("sorts newest first and keeps the hub's order for equal times", () => {
    const a = line("a", "pve-01", 1);
    const b = line("b", "pve-01", 5);
    const c = { ...line("c", "nas01", 5) };
    expect(newestFirst([a, b, c]).map((e) => e.id)).toEqual(["b", "c", "a"]);
  });

  it("takes context lines from the same device only", () => {
    const list = newestFirst([
      line("1", "pve-01", 1), line("2", "nas01", 2), line("3", "pve-01", 3), line("4", "pve-01", 4),
      line("5", "nas01", 5), line("6", "pve-01", 6), line("7", "pve-01", 7), line("8", "pve-01", 8),
    ]);
    const open = list.find((e) => e.id === "4")!;
    const { newer, older } = contextLines(list, open);
    expect(newer.map((e) => e.id)).toEqual(["7", "6"]);
    expect(older.map((e) => e.id)).toEqual(["3", "1"]);
  });

  it("copies a selection oldest first, as a log file reads", () => {
    const list = newestFirst([line("1", "pve-01", 1), line("2", "pve-01", 2), line("3", "pve-01", 3)]);
    expect(selectedText(list, new Set(["1", "3"]), (e) => e.id)).toBe("1\n3");
  });
});
