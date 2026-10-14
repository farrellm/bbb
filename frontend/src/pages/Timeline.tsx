import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { api, data, type TimelineEvent } from "../api/client";
import EventLine from "../components/EventLine";
import { dayHeading } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import { useMarkRead } from "../lib/useMarkRead";
import { useNow } from "../lib/useNow";

const PAGE = 50;

// Timeline is every change, newest first and grouped by day, read and
// unread alike.
export default function Timeline() {
  const now = useNow();
  const first = useLoad("timeline", () =>
    data(api.GET("/events", { params: { query: { limit: PAGE } } })),
  );
  const [older, setOlder] = useState<TimelineEvent[]>([]);
  const [next, setNext] = useState<number | null | undefined>(undefined);
  const [loadingMore, setLoadingMore] = useState(false);

  const events = [...(first.data?.events ?? []), ...older];
  const nextBefore =
    next === undefined ? (first.data?.nextBefore ?? null) : next;
  useMarkRead(events.filter((e) => e.unread).map((e) => e.id));

  const more = async () => {
    if (nextBefore === null) return;
    setLoadingMore(true);
    try {
      const page = await data(
        api.GET("/events", {
          params: { query: { before: nextBefore, limit: PAGE } },
        }),
      );
      setOlder((o) => [...o, ...page.events]);
      setNext(page.nextBefore);
    } finally {
      setLoadingMore(false);
    }
  };

  const days: { heading: string; events: TimelineEvent[] }[] = [];
  for (const e of events) {
    const heading = dayHeading(new Date(e.createdAt), now);
    const last = days.at(-1);
    if (last?.heading === heading) last.events.push(e);
    else days.push({ heading, events: [e] });
  }

  return (
    <>
      <Typography
        variant="h2"
        component="h2"
        sx={{ fontSize: "1.25rem", mt: 1 }}
      >
        Timeline
      </Typography>
      {first.error !== undefined && (
        <Alert severity="error" variant="outlined" sx={{ my: 2 }}>
          Couldn't load the timeline. {first.error.message}
        </Alert>
      )}
      {first.data === undefined && first.error === undefined && (
        <CircularProgress aria-label="Loading" sx={{ my: 4 }} />
      )}
      {first.data !== undefined && events.length === 0 && (
        <Typography color="text.secondary" sx={{ py: 2 }}>
          No changes yet. New items, edits and removals will show up here.
        </Typography>
      )}
      {days.map((d) => (
        <Box component="section" key={d.heading} aria-label={d.heading}>
          <Typography
            variant="h3"
            component="h3"
            sx={{ pt: 2.5, pb: 0.5, fontSize: "0.9375rem" }}
          >
            {d.heading}
          </Typography>
          {d.events.map((e) => (
            <EventLine key={e.id} event={e} />
          ))}
        </Box>
      ))}
      {nextBefore !== null && first.data !== undefined && (
        <Button
          onClick={() => void more()}
          disabled={loadingMore}
          sx={{ mt: 2 }}
        >
          Show older changes
        </Button>
      )}
    </>
  );
}
