import { Box } from "@mui/material";
import type { Mark } from "../lib/marks";

// ChangeBar is the revision bar in an item's left margin, borrowed from
// technical drawings: solid Big Blue for new, outlined for edited, nothing
// once read.
export default function ChangeBar({ mark }: { mark: Mark }) {
  if (mark === null) return null;
  return (
    <Box
      aria-hidden
      sx={{
        position: "absolute",
        left: 0,
        top: 12,
        bottom: 12,
        width: 6,
        borderRadius: 1,
        border: 2,
        borderColor: "blue.main",
        bgcolor: mark === "new" ? "blue.main" : "transparent",
      }}
    />
  );
}
