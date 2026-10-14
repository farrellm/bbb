// Dates in the app are the parent's local time. Relative wording is
// plain English; nothing is abbreviated in all caps.

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;

const monthDay = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
});
const weekdayMonthDay = new Intl.DateTimeFormat("en-US", {
  weekday: "short",
  month: "short",
  day: "numeric",
});
const weekday = new Intl.DateTimeFormat("en-US", { weekday: "short" });

// clock formats a time as "3:40pm".
export function clock(d: Date): string {
  const h = d.getHours();
  const m = d.getMinutes();
  return `${h % 12 === 0 ? 12 : h % 12}:${String(m).padStart(2, "0")}${h < 12 ? "am" : "pm"}`;
}

// startOfDay is local midnight on d's date.
function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

// daysBetween counts calendar days from a to b (negative if b is earlier).
function daysBetween(a: Date, b: Date): number {
  return Math.round(
    (startOfDay(b).getTime() - startOfDay(a).getTime()) / (24 * HOUR),
  );
}

// ago is a short relative time: "just now", "4 min ago", "3 hours ago",
// "yesterday", then a date.
export function ago(d: Date, now: Date): string {
  const ms = now.getTime() - d.getTime();
  if (ms < MINUTE) return "just now";
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min ago`;
  const days = daysBetween(d, now);
  if (days === 0) {
    const h = Math.floor(ms / HOUR);
    return h === 1 ? "an hour ago" : `${h} hours ago`;
  }
  if (days === 1) return "yesterday";
  return `on ${monthDay.format(d)}`;
}

// stamp is a weekday and time for markers: "Tue 3:40pm", or "Oct 2,
// 3:40pm" when more than a week old.
export function stamp(d: Date, now: Date): string {
  if (daysBetween(d, now) < 7) return `${weekday.format(d)} ${clock(d)}`;
  return `${monthDay.format(d)}, ${clock(d)}`;
}

// shortDate is "Oct 9".
export function shortDate(d: Date): string {
  return monthDay.format(d);
}

// dayHeading names a day for grouped lists: "Today", "Yesterday",
// "Mon, Oct 12".
export function dayHeading(d: Date, now: Date): string {
  const days = daysBetween(d, now);
  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  return weekdayMonthDay.format(d);
}

// parseDate reads a "YYYY-MM-DD" deadline as local midnight.
function parseDate(s: string): Date {
  const [y = 0, m = 1, d = 1] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}

export type Due = { text: string; urgent: boolean; past: boolean };

// due describes a deadline for the flyer's deadline tab. Urgent (shown in
// Struck) means today or within two days.
export function due(deadline: string, now: Date): Due {
  const d = parseDate(deadline);
  const days = daysBetween(now, d);
  if (days < 0)
    return { text: `Was due ${monthDay.format(d)}`, urgent: false, past: true };
  if (days === 0) return { text: "Due today", urgent: true, past: false };
  if (days === 1) return { text: "Due tomorrow", urgent: true, past: false };
  if (days < 14)
    return { text: `Due in ${days} days`, urgent: days <= 2, past: false };
  return { text: `Due ${monthDay.format(d)}`, urgent: false, past: false };
}
