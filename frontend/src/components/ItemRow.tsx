import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { Box, ButtonBase, Collapse, Link, Typography } from "@mui/material";
import { type ReactNode, useId, useState } from "react";
import { Link as RouterLink } from "react-router";
import type { Item } from "../api/client";
import { ago } from "../lib/time";
import { useDiffLink } from "../lib/diffLink";
import { wordChange } from "../lib/words";
import ChangeBar from "./ChangeBar";
import { markOf } from "../lib/marks";
import ItemBody from "./ItemBody";
import TearOffTabs from "./TearOffTabs";

type Props = { item: Item; now: Date; defaultOpen?: boolean };

// ItemRow is one item in a list: a ruled row, or a flyer for signups.
// The title toggles the body.
export default function ItemRow({ item, now, defaultOpen = false }: Props) {
  const [open, setOpen] = useState(defaultOpen);
  const bodyId = useId();
  const mark = markOf(item);
  const removed = item.removedAt !== null;
  const flyer = item.isSignup && !removed;

  return (
    <Box
      component="article"
      sx={{
        position: "relative",
        pl: 2.5,
        py: 1.5,
        borderBottom: 1,
        borderColor: "divider",
        opacity: removed ? 0.6 : 1,
      }}
    >
      <ChangeBar mark={mark} />
      <Box
        sx={
          flyer
            ? {
                bgcolor: "flyer.main",
                color: "flyer.contrastText",
                borderRadius: 1,
                px: 1.5,
                pt: 0.5,
                overflow: "hidden",
              }
            : { px: 1.5 }
        }
      >
        <Typography variant="h3" component="h3">
          <ButtonBase
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            aria-controls={bodyId}
            sx={{
              display: "flex",
              width: "100%",
              minHeight: 44,
              justifyContent: "space-between",
              alignItems: "flex-start",
              gap: 1,
              textAlign: "left",
              font: "inherit",
              py: 0.5,
              borderRadius: 1,
            }}
          >
            <Box component="span">
              <Box
                component="span"
                sx={{
                  display: "block",
                  textDecoration: removed ? "line-through" : "none",
                }}
              >
                {item.title}
              </Box>
              {item.subheader !== "" && (
                <Typography
                  component="span"
                  variant="body2"
                  color="text.secondary"
                  sx={{ display: "block", mt: 0.25, fontWeight: 400 }}
                >
                  {item.subheader}
                </Typography>
              )}
            </Box>
            <ExpandMoreIcon
              aria-hidden
              sx={{
                mt: 0.25,
                color: "text.secondary",
                transform: open ? "rotate(180deg)" : "none",
                transition: "transform 150ms",
              }}
            />
          </ButtonBase>
        </Typography>
        <Meta item={item} now={now} />
        <Collapse in={open} id={bodyId}>
          <Box sx={{ pt: 1, pb: 1.5 }}>
            <ItemBody html={item.bodyHtml} />
            {/* Opened by default only on the item's own page. */}
            {!defaultOpen && (
              <Link
                component={RouterLink}
                to={`/items/${item.panelId}`}
                variant="body2"
                sx={{ display: "inline-block", mt: 1.5, py: 1 }}
              >
                Version history
              </Link>
            )}
          </Box>
        </Collapse>
        {flyer && (
          <TearOffTabs
            links={item.signupLinks}
            deadline={item.deadline}
            now={now}
          />
        )}
      </Box>
    </Box>
  );
}

// Meta is the line under the title: what happened and when, in plain
// words, with "See changes" for edits.
function Meta({ item, now }: { item: Item; now: Date }) {
  const diffLink = useDiffLink();
  const e = item.latestEvent;
  let text: ReactNode;
  let changes: string | null = null;

  if (item.removedAt !== null) {
    text = `Removed ${ago(new Date(item.removedAt), now)}`;
  } else if (e?.kind === "edit") {
    const when = ago(new Date(e.createdAt), now);
    text = (
      <>
        <Emph on={e.unread}>Edited</Emph> {when},{" "}
        {wordChange(e.wordsAdded, e.wordsRemoved)}
      </>
    );
    if (e.fromVersionId !== null && e.toVersionId !== null)
      changes = diffLink(item.panelId, e.fromVersionId, e.toVersionId);
  } else if (e?.kind === "new" && e.unread) {
    text = (
      <>
        <Emph on>New</Emph> {ago(new Date(e.createdAt), now)}
      </>
    );
  } else {
    text = `First seen ${ago(new Date(item.firstSeenAt), now)}`;
  }

  return (
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
      <span>{text}</span>
      {changes !== null && (
        <Link
          component={RouterLink}
          to={changes}
          sx={{ color: "blue.main", fontWeight: 700, py: 1.25 }}
        >
          See changes
        </Link>
      )}
      {e?.repostOf != null && item.removedAt === null && (
        <Link
          component={RouterLink}
          to={`/items/${e.repostOf}`}
          sx={{ py: 1.25 }}
        >
          Posted before; see the earlier one
        </Link>
      )}
    </Typography>
  );
}

function Emph({ on, children }: { on: boolean; children: ReactNode }) {
  return (
    <Box
      component="span"
      sx={on ? { color: "blue.main", fontWeight: 700 } : { fontWeight: 700 }}
    >
      {children}
    </Box>
  );
}
