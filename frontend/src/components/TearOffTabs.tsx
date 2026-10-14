import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import { Box, ButtonBase } from "@mui/material";
import type { SignupLink } from "../api/client";
import { due } from "../lib/time";

type Props = { links: SignupLink[]; deadline: string | null; now: Date };

function host(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

// TearOffTabs is the bottom of a signup flyer: a perforation, then one tab
// per signup form and one for the deadline, which turns Struck within two
// days.
export default function TearOffTabs({ links, deadline, now }: Props) {
  if (links.length === 0 && deadline === null) return null;
  const d = deadline === null ? null : due(deadline, now);
  const tab = {
    minHeight: 44,
    px: 1.5,
    display: "flex",
    alignItems: "center",
    gap: 0.75,
    fontWeight: 700,
    "& + &": { borderLeft: "2px dashed", borderColor: "flyer.dark" },
  } as const;
  return (
    <Box
      sx={{
        display: "flex",
        flexWrap: "wrap",
        borderTop: "2px dashed",
        borderColor: "flyer.dark",
        mx: -1.5,
      }}
    >
      {links.map((l) => (
        <ButtonBase
          key={l.url}
          component="a"
          href={l.url}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={`${l.label}: ${l.text === "" ? host(l.url) : l.text} (opens in a new tab)`}
          sx={{ ...tab, color: "text.primary", fontSize: "0.9375rem" }}
        >
          {l.label}
          <OpenInNewIcon sx={{ fontSize: "1rem" }} />
        </ButtonBase>
      ))}
      {d !== null && (
        <Box
          sx={{
            ...tab,
            color: d.urgent ? "struck.main" : "text.secondary",
            fontWeight: d.urgent ? 700 : 400,
          }}
        >
          {d.text}
        </Box>
      )}
    </Box>
  );
}
