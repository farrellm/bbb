import { Button, Typography } from "@mui/material";
import { Link } from "react-router";
import Page from "../components/Page";

// SignedIn is where sign-in lands when the iOS home-screen app started it:
// the app picks the session up on its own.
export default function SignedIn() {
  return (
    <Page>
      <Typography variant="h1" sx={{ mt: 6 }}>
        You're signed in
      </Typography>
      <Typography sx={{ mt: 2, mb: 4 }}>
        Go back to the BBB Watch app on your Home Screen. It will pick up the
        sign-in by itself.
      </Typography>
      <Button component={Link} to="/" variant="outlined">
        Continue here instead
      </Button>
    </Page>
  );
}
