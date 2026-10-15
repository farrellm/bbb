import {
  Alert,
  Avatar,
  Box,
  Button,
  Link,
  List,
  ListItem,
  ListItemText,
  TextField,
  Typography,
} from "@mui/material";
import { Link as RouterLink } from "react-router";
import { type ReactNode, useEffect, useState } from "react";
import { api, data, ok, type Invite, type ScrapeRun } from "../api/client";
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
          Pingry BBB Watch checks the page every few minutes and keeps every
          version of every item, so you can see what changed.
        </Typography>
      </Section>
    </>
  );
}

// InviteSection lets admins make an invite link to send other parents,
// and lists the links that still work with how many places each has left.
function InviteSection() {
  const open = useLoad("invites", () => data(api.GET("/admin/invites")));
  const [fresh, setFresh] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [uses, setUses] = useState("1");

  const maxUses = parseUses(uses);
  const create = async () => {
    if (maxUses === null) return;
    setError(null);
    try {
      const inv = await data(api.POST("/admin/invites", { body: { maxUses } }));
      setFresh(inv.code);
      open.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };
  const share = async (inv: Invite) => {
    if (typeof navigator.share === "function") {
      await navigator
        .share({ title: "Pingry BBB Watch", url: inv.url })
        .catch(() => undefined);
      return;
    }
    await navigator.clipboard.writeText(inv.url);
    setCopied(inv.code);
  };
  const expire = async (inv: Invite) => {
    setError(null);
    try {
      await ok(
        api.DELETE("/admin/invites/{code}", {
          params: { path: { code: inv.code } },
        }),
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
    open.reload();
  };

  const shown = error ?? open.error?.message ?? null;
  return (
    <Section title="Invite a parent">
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        Each link works for the number of parents you choose, and expires after
        two weeks.
      </Typography>
      {shown !== null && (
        <Alert severity="error" variant="outlined" sx={{ mb: 1.5 }}>
          {shown}
        </Alert>
      )}
      <Box
        sx={{
          display: "flex",
          gap: 1.5,
          flexWrap: "wrap",
          alignItems: "start",
        }}
      >
        <TextField
          type="number"
          label="Parents"
          value={uses}
          onChange={(e) => setUses(e.target.value)}
          error={maxUses === null}
          helperText={maxUses === null ? "1 to 100" : undefined}
          slotProps={{ htmlInput: { min: 1, max: 100, inputMode: "numeric" } }}
          size="small"
          sx={{ width: 110 }}
        />
        <Button
          variant="outlined"
          disabled={maxUses === null}
          onClick={() => void create()}
          sx={{ minHeight: 40 }}
        >
          Make an invite link
        </Button>
      </Box>
      {open.data !== undefined && open.data.length > 0 && (
        <Box
          component="ul"
          aria-label="Open invite links"
          sx={{ listStyle: "none", p: 0, m: 0, mt: 2 }}
        >
          {open.data.map((inv) => (
            <InviteRow
              key={inv.code}
              invite={inv}
              fresh={inv.code === fresh}
              copied={inv.code === copied}
              onShare={() => void share(inv)}
              onExpire={() => void expire(inv)}
            />
          ))}
        </Box>
      )}
    </Section>
  );
}

// InviteRow is one open link: places left, a seat for each parent it
// admits, its dates, and buttons to send it or expire it. Expiring takes a
// second tap, since a link can't be brought back.
function InviteRow({
  invite: inv,
  fresh,
  copied,
  onShare,
  onExpire,
}: {
  invite: Invite;
  fresh: boolean;
  copied: boolean;
  onShare: () => void;
  onExpire: () => void;
}) {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const t = setTimeout(() => setArmed(false), 4000);
    return () => clearTimeout(t);
  }, [armed]);
  const left = inv.maxUses - inv.uses;
  const made = shortDate(new Date(inv.createdAt));
  return (
    <Box
      component="li"
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 1.5,
        py: 1.25,
        borderTop: 1,
        borderColor: "divider",
        // A new link arrives with a Big Blue wash that fades out.
        ...(fresh && {
          "@keyframes inviteFresh": {
            from: {
              backgroundColor:
                "color-mix(in srgb, var(--mui-palette-blue-main) 16%, transparent)",
            },
            to: { backgroundColor: "transparent" },
          },
          animation: "inviteFresh 2.4s ease-out",
        }),
      }}
    >
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography sx={{ fontWeight: 700 }}>
          {inv.maxUses === 1
            ? "For one parent"
            : `${left} of ${inv.maxUses} parents left`}
        </Typography>
        <Seats max={inv.maxUses} used={inv.uses} />
        <Typography variant="body2" color="text.secondary">
          <Nowrap>Made {made},</Nowrap>{" "}
          <Nowrap>
            {inv.expiresAt === null
              ? "never expires"
              : `expires ${shortDate(new Date(inv.expiresAt))}`}
          </Nowrap>
        </Typography>
      </Box>
      <Box sx={{ display: "flex", gap: 0.5, flexShrink: 0 }}>
        <Button
          onClick={armed ? onExpire : () => setArmed(true)}
          onBlur={() => setArmed(false)}
          sx={{ color: armed ? "struck.main" : "text.secondary" }}
        >
          {armed ? "Expire now" : "Expire"}
        </Button>
        <Button variant="outlined" onClick={onShare}>
          {copied ? "Copied" : "Share"}
        </Button>
      </Box>
    </Box>
  );
}

function Nowrap({ children }: { children: ReactNode }) {
  return (
    <Box component="span" sx={{ whiteSpace: "nowrap" }}>
      {children}
    </Box>
  );
}

function seat(taken: boolean) {
  return {
    height: 6,
    borderRadius: "1px",
    bgcolor: taken ? "text.secondary" : "text.primary",
    opacity: taken ? 0.35 : 1,
  };
}

// Seats draws a link's places like a sign-up sheet: taken ones faint,
// open ones in Ink. Past a dozen they merge into one bar.
function Seats({ max, used }: { max: number; used: number }) {
  return (
    <Box
      aria-hidden
      sx={{ display: "flex", gap: "3px", my: 0.75, maxWidth: 12 * 17 }}
    >
      {max <= 12 ? (
        Array.from({ length: max }, (_, i) => (
          <Box key={i} sx={{ ...seat(i < used), width: 14 }} />
        ))
      ) : (
        <>
          {used > 0 && <Box sx={{ ...seat(true), flex: used }} />}
          <Box sx={{ ...seat(false), flex: max - used }} />
        </>
      )}
    </Box>
  );
}

// parseUses reads the parents field, or null if it isn't a whole number the
// server accepts.
function parseUses(s: string): number | null {
  if (!/^\d+$/.test(s.trim())) return null;
  const n = Number(s);
  return n >= 1 && n <= 100 ? n : null;
}

// runOutcome says how one check of the page went.
function runOutcome(r: ScrapeRun): string {
  if (r.error !== null) return r.error;
  if (r.unchanged) return "Unchanged";
  return r.itemCount === null ? "Read the page" : `Read ${r.itemCount} items`;
}

// ChecksSection shows admins the latest checks of the page, so an alert
// that Pingry BBB Watch can't read it says why.
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
