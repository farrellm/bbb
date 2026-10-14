import { Box, CircularProgress } from "@mui/material";
import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router";
import { useAuth } from "./useAuth";

// RequireAuth sends signed-out visitors to /login, remembering where they
// were headed.
export default function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  const loc = useLocation();
  if (loading) {
    return (
      <Box sx={{ display: "grid", placeItems: "center", minHeight: "100dvh" }}>
        <CircularProgress aria-label="Loading" />
      </Box>
    );
  }
  if (!user) {
    const next = loc.pathname + loc.search;
    const to =
      next === "/" ? "/login" : `/login?next=${encodeURIComponent(next)}`;
    return <Navigate to={to} replace />;
  }
  return children;
}
