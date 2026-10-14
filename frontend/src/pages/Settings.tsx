import {
  Alert,
  Avatar,
  Box,
  Button,
  Link,
  List,
  ListItem,
  ListItemText,
  Typography,
} from "@mui/material";
import { Link as RouterLink } from "react-router";
import { useState } from "react";
import { api, data, type Invite, type ScrapeRun } from "../api/client";
import { useAuth } from "../auth/useAuth";
import NotificationSettings from "../components/NotificationSettings";
import PreferencesForm from "../components/PreferencesForm";
import Section from "../components/Section";
import { shortDate, stamp } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import { useNow } from "../lib/useNow";

export default function Settings() {
  const { user, signOut } = useAuth();
  if (!user) return null;
  return (
    <>
      <Typography
        variant="h2"
        component="h2"
        sx={{ fontSize: "1.25rem", mt: 1, mb: 2 }}
      >
        Settings
      </Typography>

      <Section title="Account">
        <Box sx={{ display: "flex", gap: 1.5, alignItems: "center", mb: 2 }}>
          <Avatar
            alt=""
            src={user.pictureUrl === "" ? undefined : user.pictureUrl}
            slotProps={{ img: { referrerPolicy: "no-referrer" } }}
          >
            {(user.name || user.email).slice(0, 1).toUpperCase()}
          </Avatar>
          <Box>
            <Typography sx={{ fontWeight: 700 }}>
              {user.name || user.email}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {user.email}
            </Typography>
          </Box>
        </Box>
        <Button variant="outlined" onClick={() => void signOut()}>
          Sign out
        </Button>
      </Section>

      <NotificationSettings />
      <PreferencesForm />

      {user.isAdmin && <InviteSection />}
      {user.isAdmin && <ChecksSection />}

      <Section title="About">
        <Typography>
          Unofficial. Reads the public Big Blue Bulletin page.{" "}
          <Link component={RouterLink} to="/privacy">
            Privacy
          </Link>
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          BBB Watch checks the page every few minutes and keeps every version of
          every item, so you can see what changed.
        </Typography>
      </Section>
    </>
  );
}

// InviteSection lets admins make a single-use invite link to send another
// parent.
function InviteSection() {
  const [invite, setInvite] = useState<Invite | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const create = async () => {
    setError(null);
    setCopied(false);
    try {
      setInvite(await data(api.POST("/admin/invites", { body: {} })));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };
  const share = async (url: string) => {
    if (typeof navigator.share === "function") {
      await navigator.share({ title: "BBB Watch", url }).catch(() => undefined);
      return;
    }
    await navigator.clipboard.writeText(url);
    setCopied(true);
  };

  return (
    <Section title="Invite a parent">
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        Each link lets one parent create an account, and expires after two
        weeks.
      </Typography>
      {error !== null && (
        <Alert severity="error" variant="outlined" sx={{ mb: 1.5 }}>
          {error}
        </Alert>
      )}
      {invite === null ? (
        <Button variant="outlined" onClick={() => void create()}>
          Make an invite link
        </Button>
      ) : (
        <>
          <Typography sx={{ overflowWrap: "anywhere", mb: 0.5 }}>
            {invite.url}
          </Typography>
          {invite.expiresAt !== null && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              Expires {shortDate(new Date(invite.expiresAt))}
            </Typography>
          )}
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            <Button variant="contained" onClick={() => void share(invite.url)}>
              {copied ? "Copied" : "Share link"}
            </Button>
            <Button onClick={() => void create()}>Make another</Button>
          </Box>
        </>
      )}
    </Section>
  );
}

// runOutcome says how one check of the page went.
function runOutcome(r: ScrapeRun): string {
  if (r.error !== null) return r.error;
  if (r.unchanged) return "Unchanged";
  return r.itemCount === null ? "Read the page" : `Read ${r.itemCount} items`;
}

// ChecksSection shows admins the latest checks of the page, so an alert
// that BBB Watch can't read it says why.
function ChecksSection() {
  const runs = useLoad("scrape-runs", () =>
    data(api.GET("/admin/scrape-runs", { params: { query: { limit: 12 } } })),
  );
  const now = useNow();
  return (
    <Section title="Recent checks">
      {runs.error !== undefined && (
        <Alert severity="error" variant="outlined">
          {runs.error.message}
        </Alert>
      )}
      {runs.data !== undefined && runs.data.length === 0 && (
        <Typography>The page hasn't been checked yet.</Typography>
      )}
      {runs.data !== undefined && runs.data.length > 0 && (
        <List dense disablePadding>
          {runs.data.map((r) => {
            const bad = r.error !== null || r.suspect;
            return (
              <ListItem key={r.id} disableGutters>
                <ListItemText
                  primary={runOutcome(r)}
                  secondary={stamp(new Date(r.startedAt), now)}
                  slotProps={{
                    primary: {
                      color: bad ? "struck.main" : undefined,
                      sx: { overflowWrap: "anywhere" },
                    },
                  }}
                />
              </ListItem>
            );
          })}
        </List>
      )}
    </Section>
  );
}
