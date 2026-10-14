import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { Alert, CircularProgress, Link, Typography } from "@mui/material";
import { Link as RouterLink, useParams, useSearchParams } from "react-router";
import { api, data } from "../api/client";
import DiffContent from "../components/DiffContent";
import { useLoad } from "../lib/useLoad";

// DiffPage is the diff as a page of its own, for deep links such as the
// ones notifications will open: /items/:panelID/diff?from=&to=.
export default function DiffPage() {
  const panelID = Number(useParams().panelID);
  const [params] = useSearchParams();
  const detail = useLoad(String(panelID), () =>
    data(api.GET("/items/{panelID}", { params: { path: { panelID } } })),
  );
  const versions = detail.data?.versions ?? [];
  // Without from/to, show the latest change.
  const to = Number(params.get("to") ?? versions.at(-1)?.id ?? 0);
  const from = Number(params.get("from") ?? versions.at(-2)?.id ?? 0);

  return (
    <>
      <Link
        component={RouterLink}
        to={`/items/${panelID}`}
        sx={{
          display: "inline-flex",
          alignItems: "center",
          gap: 0.5,
          py: 1.25,
        }}
      >
        <ArrowBackIcon fontSize="small" /> Item history
      </Link>
      {detail.error !== undefined && (
        <Alert severity="error" variant="outlined" sx={{ my: 2 }}>
          {detail.error.message}
        </Alert>
      )}
      {detail.data === undefined && detail.error === undefined && (
        <CircularProgress aria-label="Loading" sx={{ my: 4 }} />
      )}
      {detail.data !== undefined && (
        <>
          <Typography variant="h2" sx={{ mt: 1, mb: 0.5 }}>
            {detail.data.item.title}
          </Typography>
          {from > 0 && to > 0 && from !== to ? (
            <DiffContent detail={detail.data} from={from} to={to} />
          ) : (
            <Typography color="text.secondary">
              This item has only one version, so there's nothing to compare.
            </Typography>
          )}
        </>
      )}
    </>
  );
}
