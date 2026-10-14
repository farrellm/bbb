import { Box, Typography } from "@mui/material";
import { useMemo } from "react";
import { type DiffSegment, wordCount, wordDiff } from "../lib/wordDiff";

// The parts of a version a diff compares: the normalized text the server
// hashes, so the view shows exactly what counted as a change.
export type Comparable = { title: string; subheader: string; bodyText: string };

const sections = [
  { key: "title", label: "Title" },
  { key: "subheader", label: "Details" },
  { key: "bodyText", label: "Body" },
] as const;

const hidden = {
  position: "absolute",
  // Strings: in sx a bare 1 means 100%.
  width: "1px",
  height: "1px",
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
} as const;

// DiffView shows a word-level diff of two versions, section by section,
// the way nisaba's BlockAttributeDiffPage renders one: inserted words in
// Big Blue and underlined, deleted words in Struck and struck through.
// Sections that didn't change collapse to one line.
export default function DiffView({
  before,
  after,
}: {
  before: Comparable;
  after: Comparable;
}) {
  const diffs = useMemo(
    () =>
      sections.map(({ key, label }) => ({
        key,
        label,
        same: before[key] === after[key],
        segments: wordDiff(before[key], after[key]),
      })),
    [before, after],
  );
  let removed = 0;
  let added = 0;
  for (const d of diffs) {
    for (const seg of d.segments) {
      if (seg.type === "remove") removed += wordCount(seg.text);
      if (seg.type === "add") added += wordCount(seg.text);
    }
  }

  return (
    <Box>
      <Typography variant="body2" sx={{ fontWeight: 700, mb: 2 }}>
        <Box
          component="span"
          sx={{ color: removed > 0 ? "struck.main" : "text.secondary" }}
        >
          &minus;{removed} {removed === 1 ? "word" : "words"}
        </Box>
        <Box component="span" sx={{ color: "text.secondary", mx: 1 }}>
          /
        </Box>
        <Box
          component="span"
          sx={{ color: added > 0 ? "blue.main" : "text.secondary" }}
        >
          +{added} {added === 1 ? "word" : "words"}
        </Box>
      </Typography>
      {diffs.map((d) =>
        d.same ? (
          <Typography
            key={d.key}
            variant="body2"
            color="text.secondary"
            sx={{ py: 1, borderTop: 1, borderColor: "divider" }}
          >
            {d.label} unchanged
          </Typography>
        ) : (
          <Box
            component="section"
            key={d.key}
            sx={{ py: 1.5, borderTop: 1, borderColor: "divider" }}
          >
            <Typography
              variant="h2"
              component="h3"
              sx={{ fontSize: "0.8125rem", mb: 0.75 }}
            >
              {d.label}
            </Typography>
            <Segments segments={d.segments} />
          </Box>
        ),
      )}
    </Box>
  );
}

function Segments({ segments }: { segments: DiffSegment[] }) {
  return (
    <Box
      sx={{
        lineHeight: 1.7,
        whiteSpace: "pre-wrap",
        overflowWrap: "anywhere",
      }}
    >
      {/* Segments never reorder, so their index is a stable key. */}
      {segments.map((seg, i) => {
        if (seg.type === "add") {
          return (
            <Box
              component="ins"
              key={i}
              sx={{
                color: "blue.main",
                textDecoration: "underline",
                textUnderlineOffset: "0.18em",
              }}
            >
              <Box component="span" sx={hidden}>
                added:{" "}
              </Box>
              {seg.text}
            </Box>
          );
        }
        if (seg.type === "remove") {
          return (
            <Box
              component="del"
              key={i}
              sx={{ color: "struck.main", textDecoration: "line-through" }}
            >
              <Box component="span" sx={hidden}>
                removed:{" "}
              </Box>
              {seg.text}
            </Box>
          );
        }
        return <span key={i}>{seg.text}</span>;
      })}
    </Box>
  );
}
