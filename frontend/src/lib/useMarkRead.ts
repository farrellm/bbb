import { useEffect, useRef } from "react";
import { markRead } from "../api/client";

// useMarkRead marks events read once they have been on screen. The page
// keeps showing them as unread from the data it already has, so the marks
// last for this visit and are gone the next time.
export function useMarkRead(ids: number[]): void {
  const sent = useRef(new Set<number>());
  const key = ids.join(",");
  useEffect(() => {
    const fresh = key
      .split(",")
      .filter((s) => s !== "")
      .map(Number)
      .filter((id) => !sent.current.has(id));
    if (fresh.length === 0) return;
    // A beat on screen first, so a page that's immediately left (a
    // redirect, a mis-tap) doesn't clear the marks.
    const t = setTimeout(() => {
      for (const id of fresh) sent.current.add(id);
      markRead(fresh);
    }, 1500);
    return () => clearTimeout(t);
  }, [key]);
}
