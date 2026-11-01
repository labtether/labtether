import { describe, expect, it } from "vitest";
import { futureWhen, pastWhen } from "../autoTime";

// Risk: a last or next run shows the wrong day (yesterday as a clock time, last year without its year).
const now = new Date(2026, 9, 27, 22, 40); // Tue 27 Oct 2026, local time
const at = (month: number, day: number, h: number, m: number, year = 2026) => new Date(year, month, day, h, m).toISOString();

describe("automation times", () => {
  it("shows past runs as a clock time today, a weekday this week and a date before", () => {
    expect(pastWhen(at(9, 27, 2, 14), now, "en-GB")).toBe("02:14");
    expect(pastWhen(at(9, 24, 10, 4), now, "en-GB")).toBe("Sat 10:04");
    expect(pastWhen(at(9, 20, 9, 0), now, "en-GB")).toBe("20 Oct");
    expect(pastWhen(at(11, 30, 9, 0, 2025), now, "en-GB")).toBe("30 Dec 2025");
    expect(pastWhen("not a time", now, "en-GB")).toBe("");
    // A 12-hour locale drops the lead zero.
    expect(pastWhen(at(9, 27, 13, 27), now, "en-US")).toBe("13:27");
  });

  it("shows coming runs with their time on every distance", () => {
    expect(futureWhen(at(9, 27, 23, 0), now, "en-GB")).toBe("23:00");
    expect(futureWhen(at(9, 28, 2, 0), now, "en-GB")).toBe("Wed 02:00");
    expect(futureWhen(at(10, 24, 20, 0), now, "en-GB")).toBe("24 Nov 20:00");
  });
});
