import { Box, Typography } from "@mui/material";
import type { ReactNode } from "react";

// Section is one titled block of the Settings page, ruled off above.
export default function Section({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <Box
      component="section"
      sx={{ py: 2.5, borderTop: 1, borderColor: "divider" }}
    >
      <Typography variant="h2" sx={{ mb: 1.5 }}>
        {title}
      </Typography>
      {children}
    </Box>
  );
}
