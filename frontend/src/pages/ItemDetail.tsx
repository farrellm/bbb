import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Link,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { Link as RouterLink, useNavigate, useParams } from "react-router";
import { api, data } from "../api/client";
import EventLine from "../components/EventLine";
import ItemRow from "../components/ItemRow";
import { useDiffLink } from "../lib/diffLink";
import { shortDate, clock } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import { useNow } from "../lib/useNow";
import { divisionNames } from "../lib/words";

// ItemDetail is one item with its full history; any two versions can be
// compared.
export default function ItemDetail() {
  const now = useNow();
  const panelID = Number(useParams().panelID);
  const detail = useLoad(String(panelID), () =>
    data(api.GET("/items/{panelID}", { params: { path: { panelID } } })),
  );

  return (
    <>
      <Link
        component={RouterLink}
        to="/"
        sx={{
          display: "inline-flex",
          alignItems: "center",
          gap: 0.5,
          py: 1.25,
        }}
      >
        <ArrowBackIcon fontSize="small" /> Bulletin
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
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            {divisionNames[detail.data.item.division]}
          </Typography>
          <ItemRow
            key={panelID}
            item={detail.data.item}
            now={now}
            defaultOpen
          />
          <Compare
            panelID={panelID}
            versions={detail.data.versions.map((v) => ({
              id: v.id,
              label: `${shortDate(new Date(v.seenAt))}, ${clock(new Date(v.seenAt))}`,
            }))}
          />
          <Typography variant="h2" sx={{ pt: 3, pb: 0.5 }}>
            History
          </Typography>
          {detail.data.events.map((e) => (
            <EventLine key={e.id} event={e} showTitle={false} />
          ))}
          <Typography variant="body2" color="text.secondary" sx={{ py: 1.25 }}>
            First seen {shortDate(new Date(detail.data.item.firstSeenAt))},{" "}
            {clock(new Date(detail.data.item.firstSeenAt))}
          </Typography>
        </>
      )}
    </>
  );
}

// Compare picks any two versions and opens the diff sheet on them.
function Compare({
  panelID,
  versions,
}: {
  panelID: number;
  versions: { id: number; label: string }[];
}) {
  const navigate = useNavigate();
  const diffLink = useDiffLink();
  const [from, setFrom] = useState(versions.at(-2)?.id ?? 0);
  const [to, setTo] = useState(versions.at(-1)?.id ?? 0);
  if (versions.length < 2) return null;
  return (
    <Box component="section" aria-label="Compare versions" sx={{ pt: 3 }}>
      <Typography variant="h2" sx={{ pb: 1.5 }}>
        Compare versions
      </Typography>
      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          gap: 1.5,
          alignItems: "center",
        }}
      >
        <TextField
          select
          size="small"
          label="From"
          value={from}
          onChange={(e) => setFrom(Number(e.target.value))}
          sx={{ minWidth: 160 }}
        >
          {versions.map((v) => (
            <MenuItem key={v.id} value={v.id}>
              {v.label}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label="To"
          value={to}
          onChange={(e) => setTo(Number(e.target.value))}
          sx={{ minWidth: 160 }}
        >
          {versions.map((v) => (
            <MenuItem key={v.id} value={v.id}>
              {v.label}
            </MenuItem>
          ))}
        </TextField>
        <Button
          variant="outlined"
          disabled={from === to}
          onClick={() => void navigate(diffLink(panelID, from, to))}
        >
          Compare
        </Button>
      </Box>
    </Box>
  );
}
