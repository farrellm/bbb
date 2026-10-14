import { Box } from "@mui/material";
import type { ReactNode } from "react";

// Page is the single centered column every screen uses: at most about
// 66 characters wide, a 16px gutter on phones, clear of the notch.
export default function Page({ children }: { children: ReactNode }) {
  return (
    <Box
      sx={{
        maxWidth: "66ch",
        mx: "auto",
        px: 2,
        pt: "max(16px, env(safe-area-inset-top))",
        pb: "max(32px, env(safe-area-inset-bottom))",
      }}
    >
      {children}
    </Box>
  );
}
