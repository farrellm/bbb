import { Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router";

export default function NotFound() {
  return (
    <Typography sx={{ py: 4 }}>
      There's nothing here.{" "}
      <Link component={RouterLink} to="/">
        Back to the bulletin
      </Link>
    </Typography>
  );
}
