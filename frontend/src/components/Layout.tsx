import HistoryIcon from "@mui/icons-material/History";
import SettingsOutlinedIcon from "@mui/icons-material/SettingsOutlined";
import { Alert, Box, IconButton, Link, Typography } from "@mui/material";
import { useEffect } from "react";
import { Outlet, Link as RouterLink } from "react-router";
import { api, data } from "../api/client";
import { setBadge } from "../lib/push";
import { ago, clock } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import { useNow, useOnReturn } from "../lib/useNow";
import DiffSheet from "./DiffSheet";
import Page from "./Page";

// Layout is the signed-in frame: the masthead with when the page was last
// checked, the current screen, and the diff sheet that any screen can
// open.
export default function Layout() {
  const now = useNow();
  const status = useLoad("status", () => data(api.GET("/status")));
  useOnReturn(status.reload);
  const st = status.data;
  const unread = st?.unread;
  useEffect(() => {
    if (unread !== undefined) setBadge(unread);
  }, [unread]);

  return (
    <Page>
      <Box component="header" sx={{ mb: 1 }}>
        <Box sx={{ display: "flex", alignItems: "center" }}>
          <Typography variant="h1" sx={{ flex: 1 }}>
            <Link
              component={RouterLink}
              to="/"
              underline="none"
              color="inherit"
            >
              BBB Watch
            </Link>
          </Typography>
          <IconButton
            component={RouterLink}
            to="/timeline"
            aria-label="Timeline"
          >
            <HistoryIcon />
          </IconButton>
          <IconButton
            component={RouterLink}
            to="/settings"
            aria-label="Settings"
            edge="end"
          >
            <SettingsOutlinedIcon />
          </IconButton>
        </Box>
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{ minHeight: 17 }}
        >
          {st?.checkedAt != null &&
            `Checked ${ago(new Date(st.checkedAt), now)}`}
        </Typography>
        {st?.failing === true && st.checkedAt !== null && (
          <Alert severity="warning" variant="outlined" sx={{ mt: 1 }}>
            Couldn't reach pingry.org at {clock(new Date(st.checkedAt))}.
            {st.okAt !== null &&
              ` Showing the page as of ${clock(new Date(st.okAt))}.`}
          </Alert>
        )}
      </Box>
      <Box component="main">
        <Outlet />
      </Box>
      <DiffSheet />
    </Page>
  );
}
