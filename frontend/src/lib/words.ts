import type { Division, EventKind } from "../api/client";

export const divisionNames: Record<Division, string> = {
  all: "All School",
  lower: "Lower School",
  middle: "Middle School",
  upper: "Upper School",
};

export const divisionOrder: Division[] = ["all", "lower", "middle", "upper"];

// headline is how notifications and the timeline introduce an event:
// "New in Lower School".
export function headline(kind: EventKind, division: Division): string {
  const name = divisionNames[division];
  switch (kind) {
    case "new":
      return `New in ${name}`;
    case "edit":
      return `Edited in ${name}`;
    case "moved":
      return `Moved to ${name}`;
    case "removed":
      return `Removed from ${name}`;
  }
}

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

// wordChange summarizes an edit: "12 words added, 4 removed".
export function wordChange(added: number, removed: number): string {
  if (added > 0 && removed > 0)
    return `${plural(added, "word", "words")} added, ${removed} removed`;
  if (added > 0) return `${plural(added, "word", "words")} added`;
  if (removed > 0) return `${plural(removed, "word", "words")} removed`;
  return "punctuation or formatting changed";
}
