import IosShareIcon from "@mui/icons-material/IosShare";
import { Box, Typography } from "@mui/material";

// InstallGuide tells an iPhone user how to get notifications: iOS only
// allows them in an app added to the Home Screen.
export default function InstallGuide() {
  return (
    <Box>
      <Typography>
        To get notifications, add Pingry BBB Watch to your Home Screen: tap
        Share{" "}
        <IosShareIcon
          fontSize="small"
          titleAccess="(the Share icon)"
          sx={{ verticalAlign: "text-bottom" }}
        />
        , then Add to Home Screen. Then open it from there.
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
        You'll sign in once more inside the app; it keeps its own sign-in,
        separate from Safari's.
      </Typography>
    </Box>
  );
}
