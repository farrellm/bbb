import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import CloseIcon from "@mui/icons-material/Close";
import {
  Alert,
  Box,
  CircularProgress,
  Dialog,
  IconButton,
  Slide,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import type { TransitionProps } from "@mui/material/transitions";
import type { ReactElement, Ref } from "react";
import { useLocation, useNavigate } from "react-router";
import { api, data } from "../api/client";
import { diffSearch, parseDiff, type DiffTarget } from "../lib/diffLink";
import { useLoad } from "../lib/useLoad";
import DiffContent from "./DiffContent";

function SlideUp(
  props: TransitionProps & { children: ReactElement; ref?: Ref<unknown> },
) {
  return <Slide direction="up" {...props} />;
}

// DiffSheet is the diff popout: a full-height sheet sliding up from the
// bottom on phones, a dialog on wider screens. It opens whenever the URL
// has ?item=&diff=from..to, so the browser's Back closes it.
export default function DiffSheet() {
  const loc = useLocation();
  const navigate = useNavigate();
  const theme = useTheme();
  const phone = useMediaQuery(theme.breakpoints.down("sm"));
  const params = new URLSearchParams(loc.search);
  const target = parseDiff(params);

  const go = (t: DiffTarget | null) =>
    void navigate(
      { pathname: loc.pathname, search: diffSearch(params, t) },
      { replace: t !== null },
    );

  return (
    <Dialog
      open={target !== null}
      onClose={() => go(null)}
      fullScreen={phone}
      fullWidth
      maxWidth="md"
      slots={{ transition: SlideUp }}
      aria-labelledby="diff-title"
      slotProps={{ paper: { sx: phone ? {} : { height: "90dvh" } } }}
    >
      {target !== null && <SheetBody target={target} go={go} />}
    </Dialog>
  );
}

function SheetBody({
  target,
  go,
}: {
  target: DiffTarget;
  go: (t: DiffTarget | null) => void;
}) {
  const detail = useLoad(String(target.panelId), () =>
    data(
      api.GET("/items/{panelID}", {
        params: { path: { panelID: target.panelId } },
      }),
    ),
  );
  const versions = detail.data?.versions ?? [];
  const toIndex = versions.findIndex((v) => v.id === target.to);
  const step = (i: number) => {
    const from = versions[i - 1];
    const to = versions[i];
    if (from && to) go({ panelId: target.panelId, from: from.id, to: to.id });
  };

  return (
    <Box
      sx={{
        px: 2,
        pt: "max(12px, env(safe-area-inset-top))",
        pb: "max(24px, env(safe-area-inset-bottom))",
        overflowY: "auto",
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, mb: 1 }}>
        <IconButton
          aria-label="Earlier change"
          disabled={toIndex < 2}
          onClick={() => step(toIndex - 1)}
        >
          <ChevronLeftIcon />
        </IconButton>
        <IconButton
          aria-label="Later change"
          disabled={toIndex < 0 || toIndex >= versions.length - 1}
          onClick={() => step(toIndex + 1)}
        >
          <ChevronRightIcon />
        </IconButton>
        <Box sx={{ flex: 1 }} />
        <IconButton aria-label="Close" onClick={() => go(null)}>
          <CloseIcon />
        </IconButton>
      </Box>
      {detail.error !== undefined ? (
        <Alert severity="error" variant="outlined">
          Couldn't load this item. {detail.error.message}
        </Alert>
      ) : detail.data === undefined ? (
        <CircularProgress aria-label="Loading" />
      ) : (
        <>
          <Typography id="diff-title" variant="h2" sx={{ mb: 0.5 }}>
            {detail.data.item.title}
          </Typography>
          <DiffContent detail={detail.data} from={target.from} to={target.to} />
        </>
      )}
    </Box>
  );
}
