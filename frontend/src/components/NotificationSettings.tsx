import DeleteOutlineIcon from "@mui/icons-material/DeleteOutlined";
import {
  Alert,
  Box,
  Button,
  IconButton,
  List,
  ListItem,
  ListItemText,
  Snackbar,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { api, data, type Device, ok } from "../api/client";
import { deviceName, thisDevice } from "../lib/device";
import {
  currentSubscription,
  disablePush,
  enablePush,
  isIOS,
  PermissionDenied,
  pushSupport,
} from "../lib/push";
import { shortDate } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import InstallGuide from "./InstallGuide";
import Section from "./Section";

const support = pushSupport();
const here = thisDevice(navigator.userAgent);

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// NotificationSettings turns push on or off for this device, sends a test,
// and lists every device getting the user's notifications.
export default function NotificationSettings() {
  const devices = useLoad("devices", () => data(api.GET("/push/devices")));
  const sub = useLoad(support === "supported" ? "subscription" : null, () =>
    currentSubscription().then((s) => s?.endpoint ?? null),
  );
  const [permission, setPermission] = useState(() =>
    support === "supported" ? Notification.permission : "default",
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  const endpoint = sub.data ?? null;
  const on =
    endpoint !== null &&
    (devices.data ?? []).some((d) => d.endpoint === endpoint);

  const run = async (act: () => Promise<string | null>) => {
    setBusy(true);
    setError(null);
    try {
      const done = await act();
      if (done !== null) setToast(done);
    } catch (err) {
      if (!(err instanceof PermissionDenied)) setError(message(err));
    } finally {
      if (support === "supported") setPermission(Notification.permission);
      setBusy(false);
      devices.reload();
      sub.reload();
    }
  };

  const turnOn = () =>
    run(async () => {
      await enablePush();
      return `Notifications turned on for ${here}`;
    });
  const turnOff = () =>
    run(async () => {
      await disablePush();
      return `Notifications turned off for ${here}`;
    });
  const test = () =>
    run(async () => {
      const res = await data(api.POST("/push/test"));
      if (res.sent === 0)
        throw new Error(
          "Couldn't reach any of your devices. Turn notifications off and on again on each one.",
        );
      return res.failed === 0
        ? "Test sent. It should arrive in a few seconds."
        : `Test sent to ${res.sent} of ${res.sent + res.failed} devices.`;
    });
  const remove = (d: Device) =>
    run(async () => {
      if (d.endpoint === endpoint) {
        await disablePush();
      } else {
        await ok(
          api.DELETE("/push/devices/{id}", {
            params: { path: { id: d.id } },
          }),
        );
      }
      return null;
    });

  return (
    <Section title="Notifications">
      {error !== null && (
        <Alert severity="error" variant="outlined" sx={{ mb: 1.5 }}>
          {error}
        </Alert>
      )}
      {support === "needs-install" && <InstallGuide />}
      {support === "unsupported" && (
        <Typography>
          This browser can't show notifications. On an iPhone, open BBB Watch
          from your Home Screen.
        </Typography>
      )}
      {support === "supported" && permission === "denied" && (
        <Alert severity="info" variant="outlined" sx={{ mb: 1.5 }}>
          {isIOS()
            ? "Notifications are blocked for BBB Watch. To allow them, open the Settings app, tap Notifications, then BBB Watch, and turn on Allow Notifications."
            : "Notifications are blocked for this site. Allow them in your browser's site settings, then come back."}
        </Alert>
      )}
      {support === "supported" && permission !== "denied" && (
        <>
          <Typography sx={{ mb: 1.5 }}>
            {on
              ? `Notifications are on for ${here}.`
              : "Get a notification when something is posted or changed."}
          </Typography>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            {on ? (
              <>
                <Button
                  variant="outlined"
                  disabled={busy}
                  onClick={() => void test()}
                >
                  Send a test
                </Button>
                <Button disabled={busy} onClick={() => void turnOff()}>
                  Turn off
                </Button>
              </>
            ) : (
              <Button
                variant="contained"
                disabled={busy || sub.loading}
                onClick={() => void turnOn()}
              >
                Turn on notifications
              </Button>
            )}
          </Box>
        </>
      )}

      {devices.data !== undefined && devices.data.length > 0 && (
        <>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2.5 }}>
            Devices getting your notifications
          </Typography>
          <List dense disablePadding>
            {devices.data.map((d) => {
              const name = deviceName(d.userAgent);
              const added = `added ${shortDate(new Date(d.createdAt))}`;
              return (
                <ListItem
                  key={d.id}
                  disableGutters
                  secondaryAction={
                    <IconButton
                      edge="end"
                      aria-label={`Remove ${name}`}
                      disabled={busy}
                      onClick={() => void remove(d)}
                    >
                      <DeleteOutlineIcon />
                    </IconButton>
                  }
                >
                  <ListItemText
                    primary={name}
                    secondary={
                      d.endpoint === endpoint
                        ? `This device, ${added}`
                        : added.charAt(0).toUpperCase() + added.slice(1)
                    }
                  />
                </ListItem>
              );
            })}
          </List>
        </>
      )}

      <Snackbar
        open={toast !== null}
        autoHideDuration={4000}
        onClose={() => setToast(null)}
        message={toast}
      />
    </Section>
  );
}
