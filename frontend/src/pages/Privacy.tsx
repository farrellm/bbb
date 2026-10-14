import { Box, Link, Typography } from "@mui/material";
import type { ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import Page from "../components/Page";

// Privacy is public: Google's consent screen links to it.
export default function Privacy() {
  return (
    <Page>
      <Typography variant="h1" sx={{ mt: 4, mb: 1 }}>
        Privacy
      </Typography>
      <Typography color="text.secondary" sx={{ mb: 3 }}>
        Pingry BBB Watch is an unofficial app, not affiliated with The Pingry
        School. It reads the public Big Blue Bulletin page and shows invited
        parents what changed.
      </Typography>

      <Section title="What it stores about you">
        When you sign in with Google, Pingry BBB Watch receives and keeps your
        Google account ID, name, email address and profile picture link. It also
        records when you last signed in, which updates you have seen, and any
        invite links you create. It does not see your Google password and asks
        for no other Google data.
      </Section>
      <Section title="How it's used">
        Only to sign you in, show your name in the app, keep track of what you
        haven't seen yet, and let admins manage who has access. There are no ads
        and no analytics.
      </Section>
      <Section title="Sharing">
        Your information is not sold or shared with anyone. It is stored on the
        server that runs Pingry BBB Watch.
      </Section>
      <Section title="Cookies">
        One cookie keeps you signed in, plus a short-lived one during sign-in.
        Neither is used for tracking.
      </Section>
      <Section title="Deleting your account">
        Ask the parent who invited you, or the app's administrator, and your
        account and everything stored with it will be deleted.
      </Section>

      <Link
        component={RouterLink}
        to="/"
        sx={{ display: "inline-block", py: 1.25 }}
      >
        Back to Pingry BBB Watch
      </Link>
    </Page>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box component="section" sx={{ mb: 2.5 }}>
      <Typography variant="h2" sx={{ mb: 0.75 }}>
        {title}
      </Typography>
      <Typography>{children}</Typography>
    </Box>
  );
}
