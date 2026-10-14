import { Box, Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router";
import type { TimelineEvent } from "../api/client";
import { useDiffLink } from "../lib/diffLink";
import { clock } from "../lib/time";
import { headline, wordChange } from "../lib/words";
import ChangeBar from "./ChangeBar";

// EventLine is one event in a history: what happened, to which item, and
// for edits a link to the changes.
export default function EventLine({
  event: e,
  showTitle = true,
}: {
  event: TimelineEvent;
  showTitle?: boolean;
}) {
  const diffLink = useDiffLink();
  const mark = !e.unread
    ? null
    : e.kind === "new"
      ? "new"
      : e.kind === "edit"
        ? "edited"
        : null;
  return (
    <Box
      sx={{
        position: "relative",
        pl: 2.5,
        py: 1.25,
        borderBottom: 1,
        borderColor: "divider",
      }}
    >
      <ChangeBar mark={mark} />
      <Typography variant="body2" color="text.secondary">
        {headline(e.kind, e.division)} at {clock(new Date(e.createdAt))}
      </Typography>
      {showTitle && (
        <Typography variant="h3" component="p" sx={{ my: 0.25 }}>
          <Link
            component={RouterLink}
            to={`/items/${e.panelId}`}
            color="inherit"
            underline="hover"
          >
            {e.title}
          </Link>
        </Typography>
      )}
      {e.kind === "edit" && (
        <Typography
          variant="body2"
          color="text.secondary"
          component="div"
          sx={{
            display: "flex",
            flexWrap: "wrap",
            columnGap: 2,
            alignItems: "center",
          }}
        >
          <span>
            {wordChange(e.wordsAdded, e.wordsRemoved)}
            {e.becameSignup && ", and it's now a sign-up"}
          </span>
          {e.fromVersionId !== null && e.toVersionId !== null && (
            <Link
              component={RouterLink}
              to={diffLink(e.panelId, e.fromVersionId, e.toVersionId)}
              sx={{ color: "blue.main", fontWeight: 700, py: 1.25 }}
            >
              See changes
            </Link>
          )}
        </Typography>
      )}
    </Box>
  );
}
