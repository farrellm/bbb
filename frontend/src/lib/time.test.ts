import { describe, expect, it } from "vitest";
import { ago, clock, dayHeading, due, stamp } from "./time";

const now = new Date(2026, 9, 14, 15, 40); // Wed Oct 14 2026, 3:40pm

describe("clock", () => {
  it("uses am/pm without a space", () => {
    expect(clock(now)).toBe("3:40pm");
    expect(clock(new Date(2026, 9, 14, 0, 5))).toBe("12:05am");
    expect(clock(new Date(2026, 9, 14, 12, 0))).toBe("12:00pm");
  });
});

describe("ago", () => {
  it("counts minutes, then hours, then days", () => {
    expect(ago(new Date(now.getTime() - 20_000), now)).toBe("just now");
    expect(ago(new Date(now.getTime() - 4 * 60_000), now)).toBe("4 min ago");
    expect(ago(new Date(2026, 9, 14, 14, 30), now)).toBe("an hour ago");
    expect(ago(new Date(2026, 9, 14, 9, 0), now)).toBe("6 hours ago");
    expect(ago(new Date(2026, 9, 13, 23, 0), now)).toBe("yesterday");
    expect(ago(new Date(2026, 9, 9, 9, 0), now)).toBe("on Oct 9");
  });
});

describe("stamp", () => {
  it("names the weekday within a week", () => {
    expect(stamp(new Date(2026, 9, 13, 15, 40), now)).toBe("Tue 3:40pm");
    expect(stamp(new Date(2026, 9, 2, 8, 5), now)).toBe("Oct 2, 8:05am");
  });
});

describe("dayHeading", () => {
  it("says today and yesterday", () => {
    expect(dayHeading(now, now)).toBe("Today");
    expect(dayHeading(new Date(2026, 9, 13), now)).toBe("Yesterday");
    expect(dayHeading(new Date(2026, 9, 12), now)).toBe("Mon, Oct 12");
  });
});

describe("due", () => {
  it("is urgent within two days", () => {
    expect(due("2026-10-14", now)).toEqual({
      text: "Due today",
      urgent: true,
      past: false,
    });
    expect(due("2026-10-15", now).text).toBe("Due tomorrow");
    expect(due("2026-10-16", now)).toEqual({
      text: "Due in 2 days",
      urgent: true,
      past: false,
    });
    expect(due("2026-10-17", now)).toEqual({
      text: "Due in 3 days",
      urgent: false,
      past: false,
    });
  });
  it("shows a date when far off or past", () => {
    expect(due("2026-11-20", now).text).toBe("Due Nov 20");
    expect(due("2026-10-02", now)).toEqual({
      text: "Was due Oct 2",
      urgent: false,
      past: true,
    });
  });
});
