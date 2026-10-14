import { useCallback } from "react";
import { useLocation } from "react-router";

// The diff sheet opens over whatever page is showing, driven by the URL
// (?item=<panel>&diff=<from>..<to>), so Back closes it and a link can
// deep-link to it.

export type DiffTarget = { panelId: number; from: number; to: number };

export function parseDiff(params: URLSearchParams): DiffTarget | null {
  const item = Number(params.get("item"));
  const m = /^(\d+)\.\.(\d+)$/.exec(params.get("diff") ?? "");
  if (!Number.isSafeInteger(item) || item <= 0 || m === null) return null;
  return { panelId: item, from: Number(m[1]), to: Number(m[2]) };
}

export function diffSearch(
  params: URLSearchParams,
  t: DiffTarget | null,
): string {
  const q = new URLSearchParams(params);
  if (t === null) {
    q.delete("item");
    q.delete("diff");
  } else {
    q.set("item", String(t.panelId));
    q.set("diff", `${t.from}..${t.to}`);
  }
  const s = q.toString();
  return s === "" ? "" : `?${s}`;
}

// useDiffLink returns a function building a link that opens the diff sheet
// over the current page.
export function useDiffLink(): (
  panelId: number,
  from: number,
  to: number,
) => string {
  const loc = useLocation();
  return useCallback(
    (panelId: number, from: number, to: number) =>
      loc.pathname +
      diffSearch(new URLSearchParams(loc.search), { panelId, from, to }),
    [loc.pathname, loc.search],
  );
}
