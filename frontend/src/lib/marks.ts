import type { Item } from "../api/client";

// A Mark is what an item's margin bar shows: unread new, unread edit, or
// nothing.
export type Mark = "new" | "edited" | null;

export function markOf(item: Item): Mark {
  const e = item.latestEvent;
  if (e === null || !e.unread) return null;
  if (e.kind === "new") return "new";
  if (e.kind === "edit") return "edited";
  return null;
}
