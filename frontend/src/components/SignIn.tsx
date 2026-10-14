import { Alert, Box, Button, Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { useAuth } from "../auth/useAuth";
import { isStandalone } from "../lib/standalone";
import Page from "./Page";
import { signInErrors as errors } from "../lib/signInErrors";

const POLL_MS = 2000;

type Props = {
  intro: string;
  error: string | null;
  invite?: string;
  next?: string;
  disabled?: boolean;
};

// SignIn is the "Sign in with Google" screen. In a browser tab it is a
// plain redirect. In the iOS home-screen app, sign-in finishes in Safari,
// whose cookies the app can't see, so the app opens sign-in with a handoff
// nonce and polls until the server hands it the session (DESIGN.md, "PWA
// and iOS specifics").
export default function SignIn({
  intro,
  error,
  invite,
  next,
  disabled,
}: Props) {
  const { signedIn } = useAuth();
  const [nonce, setNonce] = useState<string | null>(null);
  const [problem, setProblem] = useState<string | null>(null);

  const startURL = (handoff?: string) => {
    const q = new URLSearchParams();
    if (invite !== undefined) q.set("invite", invite);
    if (next !== undefined) q.set("next", next);
    if (handoff !== undefined) q.set("handoff", handoff);
    const s = q.toString();
    return `/api/auth/google/start${s === "" ? "" : `?${s}`}`;
  };

  useEffect(() => {
    if (nonce === null) return;
    let busy = false;
    const poll = async () => {
      if (busy) return;
      busy = true;
      try {
        const { data, response } = await api.GET("/auth/handoff/{nonce}", {
          params: { path: { nonce } },
        });
        if (response.status === 200 && data !== undefined && "id" in data) {
          setNonce(null);
          // The page this is on redirects once there is a user.
          signedIn(data);
        } else if (response.status === 404) {
          setNonce(null);
          setProblem(errors.handoff_expired ?? null);
        }
      } finally {
        busy = false;
      }
    };
    const id = setInterval(() => void poll(), POLL_MS);
    // Coming back from Safari is the likeliest moment it has finished.
    const onReturn = () => {
      if (document.visibilityState === "visible") void poll();
    };
    document.addEventListener("visibilitychange", onReturn);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", onReturn);
    };
  }, [nonce, signedIn]);

  const start = async () => {
    setProblem(null);
    if (!isStandalone()) {
      window.location.assign(startURL());
      return;
    }
    const { data } = await api.POST("/auth/handoff");
    if (data === undefined) {
      setProblem(errors.failed ?? null);
      return;
    }
    setNonce(data.nonce);
    window.open(startURL(data.nonce), "_blank", "noopener");
  };

  const shown = problem ?? error;
  return (
    <Page>
      <Box component="header" sx={{ mt: 6, mb: 4 }}>
        <Typography variant="h1">BBB Watch</Typography>
        <Typography color="text.secondary" sx={{ mt: 1 }}>
          {intro}
        </Typography>
      </Box>
      {shown !== null && (
        <Alert severity="error" variant="outlined" sx={{ mb: 3 }}>
          {shown}
        </Alert>
      )}
      {nonce === null ? (
        <Button
          variant="contained"
          size="large"
          onClick={() => void start()}
          disabled={disabled === true}
        >
          Sign in with Google
        </Button>
      ) : (
        <Box>
          <Typography sx={{ mb: 2 }}>
            Finish signing in with Google, then come back to this app. It will
            notice on its own.
          </Typography>
          <Button variant="outlined" onClick={() => setNonce(null)}>
            Start over
          </Button>
        </Box>
      )}
      <Typography variant="body2" color="text.secondary" sx={{ mt: 6 }}>
        Unofficial. Reads the public Big Blue Bulletin page.{" "}
        <Link component={RouterLink} to="/privacy" color="inherit">
          Privacy
        </Link>
      </Typography>
    </Page>
  );
}
