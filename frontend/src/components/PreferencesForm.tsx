import {
  Alert,
  Box,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormGroup,
  FormHelperText,
  FormLabel,
  MenuItem,
  Switch,
  TextField,
} from "@mui/material";
import { useRef, useState } from "react";
import { api, data, type Division, type Preferences } from "../api/client";
import { useLoad } from "../lib/useLoad";
import { divisionNames, divisionOrder } from "../lib/words";
import Section from "./Section";

const editSizes = [
  { words: 1, label: "Any change of a word or more" },
  { words: 5, label: "5 words or more" },
  { words: 10, label: "10 words or more" },
  { words: 25, label: "25 words or more" },
];

// Quiet hours a parent turning them on starts from.
const defaultQuiet = { start: "21:30", end: "07:00" };

// PreferencesForm is what notifies the user. Every change saves at once.
export default function PreferencesForm() {
  const loaded = useLoad("preferences", () => data(api.GET("/preferences")));
  // Edits since the last load; the server's answer replaces them.
  const [edited, setEdited] = useState<Preferences | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Only the latest save's answer counts when toggles come quickly.
  const latest = useRef(0);
  const prefs = edited ?? loaded.data;

  if (loaded.error !== undefined)
    return (
      <Section title="What notifies you">
        <Alert severity="error" variant="outlined">
          {loaded.error.message}
        </Alert>
      </Section>
    );
  if (prefs === undefined) return null;

  const save = (next: Preferences) => {
    const n = ++latest.current;
    setEdited(next);
    setError(null);
    data(api.PUT("/preferences", { body: next })).then(
      (saved) => {
        if (n === latest.current) setEdited(saved);
      },
      (err: unknown) => {
        if (n !== latest.current) return;
        setError(err instanceof Error ? err.message : String(err));
        setEdited(null);
        loaded.reload();
      },
    );
  };
  const set = <K extends keyof Preferences>(k: K, v: Preferences[K]) =>
    save({ ...prefs, [k]: v });
  const toggleDivision = (d: Division, on: boolean) =>
    set(
      "divisions",
      divisionOrder.filter((x) => (x === d ? on : prefs.divisions.includes(x))),
    );
  const sizes = editSizes.some((s) => s.words === prefs.minEditWords)
    ? editSizes
    : [
        ...editSizes,
        {
          words: prefs.minEditWords,
          label: `${prefs.minEditWords} words or more`,
        },
      ].toSorted((a, b) => a.words - b.words);

  return (
    <Section title="What notifies you">
      {error !== null && (
        <Alert severity="error" variant="outlined" sx={{ mb: 1.5 }}>
          {error}
        </Alert>
      )}

      <FormControl component="fieldset" sx={{ display: "block", mb: 2 }}>
        <FormLabel component="legend">Divisions</FormLabel>
        <FormGroup>
          {divisionOrder.map((d) => (
            <FormControlLabel
              key={d}
              label={divisionNames[d]}
              control={
                <Checkbox
                  checked={prefs.divisions.includes(d)}
                  onChange={(e) => toggleDivision(d, e.target.checked)}
                />
              }
            />
          ))}
        </FormGroup>
      </FormControl>

      <FormControl component="fieldset" sx={{ display: "block", mb: 2 }}>
        <FormLabel component="legend">Changes</FormLabel>
        <FormGroup>
          <FormControlLabel
            label="New items"
            control={
              <Switch
                checked={prefs.notifyNew}
                onChange={(e) => set("notifyNew", e.target.checked)}
              />
            }
          />
          <FormControlLabel
            label="Edits"
            control={
              <Switch
                checked={prefs.notifyEdit}
                onChange={(e) => set("notifyEdit", e.target.checked)}
              />
            }
          />
          <FormControlLabel
            label="Removed items"
            control={
              <Switch
                checked={prefs.notifyRemoved}
                onChange={(e) => set("notifyRemoved", e.target.checked)}
              />
            }
          />
        </FormGroup>
      </FormControl>

      <TextField
        select
        label="Smallest edit to notify about"
        value={prefs.minEditWords}
        disabled={!prefs.notifyEdit}
        onChange={(e) => set("minEditWords", Number(e.target.value))}
        helperText="An edit that adds a sign-up link always notifies."
        sx={{ mb: 2.5, minWidth: 260 }}
      >
        {sizes.map((s) => (
          <MenuItem key={s.words} value={s.words}>
            {s.label}
          </MenuItem>
        ))}
      </TextField>

      <FormControl component="fieldset" sx={{ display: "block" }}>
        <FormLabel component="legend">Sign-ups</FormLabel>
        <FormGroup>
          <FormControlLabel
            label="Only notify me about sign-ups"
            control={
              <Switch
                checked={prefs.signupsOnly}
                onChange={(e) => set("signupsOnly", e.target.checked)}
              />
            }
          />
          <FormControlLabel
            label="Sign-ups from every division"
            control={
              <Switch
                checked={prefs.signupsAnyDivision}
                onChange={(e) => set("signupsAnyDivision", e.target.checked)}
              />
            }
          />
        </FormGroup>
        <FormHelperText>
          Includes sign-ups in divisions you haven't picked above.
        </FormHelperText>
      </FormControl>

      <QuietHours
        // Remount on a reload, so the fields show what was saved.
        key={`${loaded.data?.quietStart}-${loaded.data?.quietEnd}`}
        start={prefs.quietStart}
        end={prefs.quietEnd}
        onChange={(start, end) =>
          save({ ...prefs, quietStart: start, quietEnd: end })
        }
      />
    </Section>
  );
}

// QuietHours turns quiet hours on and off and sets them. The time fields
// keep their own text while a time is half typed, and save once both are
// whole and different.
function QuietHours({
  start,
  end,
  onChange,
}: {
  start: string | null;
  end: string | null;
  onChange: (start: string | null, end: string | null) => void;
}) {
  const [from, setFrom] = useState(start ?? defaultQuiet.start);
  const [to, setTo] = useState(end ?? defaultQuiet.end);
  const on = start !== null && end !== null;
  const whole = /^\d\d:\d\d$/;

  const edit = (nextFrom: string, nextTo: string) => {
    setFrom(nextFrom);
    setTo(nextTo);
    if (whole.test(nextFrom) && whole.test(nextTo) && nextFrom !== nextTo)
      onChange(nextFrom, nextTo);
  };

  return (
    <FormControl component="fieldset" sx={{ display: "block", mt: 2.5 }}>
      <FormLabel component="legend">Quiet hours</FormLabel>
      <FormControlLabel
        label="Hold notifications at set times"
        control={
          <Switch
            checked={on}
            onChange={(e) =>
              e.target.checked && from !== to
                ? onChange(from, to)
                : onChange(null, null)
            }
          />
        }
      />
      {on && (
        <Box sx={{ display: "flex", gap: 1.5, mt: 1, flexWrap: "wrap" }}>
          <TextField
            type="time"
            label="From"
            value={from}
            onChange={(e) => edit(e.target.value, to)}
            error={from === to}
            sx={{ width: 150 }}
          />
          <TextField
            type="time"
            label="Until"
            value={to}
            onChange={(e) => edit(from, e.target.value)}
            error={from === to}
            sx={{ width: 150 }}
          />
        </Box>
      )}
      <FormHelperText>
        {from === to && on
          ? "Pick a different start and end."
          : "Anything posted in these hours arrives as one notification when they end. Times are the school's (Eastern)."}
      </FormHelperText>
    </FormControl>
  );
}
