import { Alert, CircularProgress, Typography } from "@mui/material";
import { api, data, type ItemEvent } from "../api/client";
import type { components } from "../api/schema";
import { shortDate } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import { useMarkRead } from "../lib/useMarkRead";
import DiffView from "./DiffView";

type Detail = components["schemas"]["ItemDetail"];

// DiffContent loads two versions of an item and diffs them, under an
// "Oct 9 → Oct 14" header. Viewing an edit marks it read.
export default function DiffContent({
  detail,
  from,
  to,
}: {
  detail: Detail;
  from: number;
  to: number;
}) {
  const panelID = detail.item.panelId;
  const pair = useLoad(`${panelID}:${from}..${to}`, () =>
    Promise.all(
      [from, to].map((versionID) =>
        data(
          api.GET("/items/{panelID}/versions/{versionID}", {
            params: { path: { panelID, versionID } },
          }),
        ),
      ),
    ),
  );
  const seen = detail.events.filter(
    (e: ItemEvent) => e.unread && e.toVersionId === to,
  );
  useMarkRead(seen.map((e) => e.id));

  if (pair.error !== undefined) {
    return (
      <Alert severity="error" variant="outlined">
        Couldn't load these versions. {pair.error.message}
      </Alert>
    );
  }
  const [before, after] = pair.data ?? [];
  if (pair.loading || before === undefined || after === undefined) {
    return <CircularProgress aria-label="Loading" />;
  }
  return (
    <>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {shortDate(new Date(before.seenAt))} &rarr;{" "}
        {shortDate(new Date(after.seenAt))}
      </Typography>
      <DiffView before={before} after={after} />
    </>
  );
}
