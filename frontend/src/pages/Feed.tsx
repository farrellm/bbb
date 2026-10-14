import {
  Alert,
  Badge,
  Box,
  Button,
  CircularProgress,
  Tab,
  Tabs,
  Typography,
} from "@mui/material";
import { type ReactNode, useCallback, useMemo } from "react";
import { useSearchParams } from "react-router";
import { api, data, type Division, type Item } from "../api/client";
import ItemRow from "../components/ItemRow";
import { markOf } from "../lib/marks";
import { stamp, due } from "../lib/time";
import { useLoad } from "../lib/useLoad";
import { useMarkRead } from "../lib/useMarkRead";
import { useNow, useOnReturn } from "../lib/useNow";
import { divisionNames, divisionOrder } from "../lib/words";

type TabKey = "everything" | Division | "signups";

const tabs: { key: TabKey; label: string }[] = [
  { key: "everything", label: "All" },
  { key: "lower", label: "Lower" },
  { key: "middle", label: "Middle" },
  { key: "upper", label: "Upper" },
  { key: "signups", label: "Sign-ups" },
];

function isTab(s: string | null): s is TabKey {
  return tabs.some((t) => t.key === s);
}

// Feed answers "what changed since I last looked, and what do I need to
// sign up for?" Unread items come first under a "since you looked" marker;
// the rest follow in page order.
export default function Feed() {
  const now = useNow();
  const [params, setParams] = useSearchParams();
  const tabParam = params.get("tab");
  const tab: TabKey = isTab(tabParam) ? tabParam : "everything";
  const showRemoved = params.get("removed") === "1";

  const feed = useLoad(`items:${showRemoved}`, () =>
    data(
      api.GET("/items", {
        params: { query: showRemoved ? { include_removed: "1" } : {} },
      }),
    ),
  );
  // Reopening the app later shows what arrived meanwhile.
  const { reload } = feed;
  useOnReturn(useCallback(() => reload(), [reload]));

  const items = useMemo(() => feed.data?.items ?? [], [feed.data]);
  const shown = useMemo(() => select(items, tab, now), [items, tab, now]);
  const unread = shown.filter((it) => markOf(it) !== null);
  const rest = shown.filter((it) => markOf(it) === null);
  useMarkRead(
    shown.flatMap((it) =>
      it.latestEvent?.unread === true ? [it.latestEvent.id] : [],
    ),
  );

  const openSignups = items.filter(
    (it) =>
      it.isSignup &&
      it.removedAt === null &&
      !(it.deadline !== null && due(it.deadline, now).past),
  ).length;
  const lastRead = feed.data?.lastReadAt ?? null;
  // "you looked, Tue 3:40pm", or "you joined" before anything was read.
  const since =
    lastRead === null
      ? "you joined"
      : `you looked, ${stamp(new Date(lastRead), now)}`;

  const setTab = (key: TabKey) => {
    const q = new URLSearchParams(params);
    if (key === "everything") q.delete("tab");
    else q.set("tab", key);
    setParams(q, { replace: true });
  };
  const toggleRemoved = () => {
    const q = new URLSearchParams(params);
    if (showRemoved) q.delete("removed");
    else q.set("removed", "1");
    setParams(q, { replace: true });
  };

  return (
    <>
      <Tabs
        value={tab}
        onChange={(_, v: TabKey) => setTab(v)}
        variant="scrollable"
        scrollButtons={false}
        aria-label="Divisions"
        sx={{ borderBottom: 1, borderColor: "divider", mx: -2, px: 1 }}
      >
        {tabs.map((t) => (
          <Tab
            key={t.key}
            value={t.key}
            label={
              t.key === "signups" && openSignups > 0 ? (
                <Badge
                  badgeContent={openSignups}
                  color="primary"
                  sx={{
                    "& .MuiBadge-badge": {
                      position: "static",
                      transform: "none",
                      ml: 0.75,
                    },
                  }}
                >
                  {t.label}
                </Badge>
              ) : (
                t.label
              )
            }
          />
        ))}
      </Tabs>

      {feed.error !== undefined && (
        <Alert severity="error" variant="outlined" sx={{ my: 2 }}>
          Couldn't load the bulletin. {feed.error.message}
        </Alert>
      )}
      {feed.data === undefined && feed.error === undefined && (
        <Box sx={{ py: 4, display: "grid", placeItems: "center" }}>
          <CircularProgress aria-label="Loading" />
        </Box>
      )}

      {feed.data !== undefined && tab === "signups" && (
        <List
          items={shown}
          now={now}
          empty="No sign-ups on the bulletin right now."
        />
      )}

      {feed.data !== undefined && tab !== "signups" && (
        <>
          {unread.length > 0 ? (
            <>
              <Marker>Since {since}</Marker>
              <List items={unread} now={now} />
              <Marker>Earlier</Marker>
            </>
          ) : (
            <Typography color="text.secondary" sx={{ py: 2 }}>
              Nothing new
              {tab === "everything"
                ? ""
                : ` in ${divisionNames[tab]}`} since {since}.
            </Typography>
          )}
          {tab === "everything" ? (
            divisionOrder.map((d) => {
              const group = rest.filter((it) => it.division === d);
              if (group.length === 0) return null;
              return (
                <Box component="section" key={d} aria-label={divisionNames[d]}>
                  <Typography variant="h2" sx={{ pt: 3, pb: 0.5 }}>
                    {divisionNames[d]}
                  </Typography>
                  <List items={group} now={now} />
                </Box>
              );
            })
          ) : (
            <List
              items={rest}
              now={now}
              empty={
                unread.length > 0
                  ? undefined
                  : `Nothing posted in ${divisionNames[tab]} right now.`
              }
            />
          )}
        </>
      )}

      {feed.data !== undefined && (
        <Button onClick={toggleRemoved} sx={{ mt: 2 }}>
          {showRemoved ? "Hide removed items" : "Show removed items"}
        </Button>
      )}
    </>
  );
}

// select picks a tab's items: a division in page order, or signups by
// deadline (soonest first, undated last, past deadlines after those).
function select(items: Item[], tab: TabKey, now: Date): Item[] {
  if (tab === "everything") return items;
  if (tab !== "signups") return items.filter((it) => it.division === tab);
  const rank = (it: Item) => {
    if (it.deadline === null) return [1, 0] as const;
    const d = due(it.deadline, now);
    return [d.past ? 2 : 0, Date.parse(it.deadline)] as const;
  };
  return items
    .filter((it) => it.isSignup)
    .toSorted((a, b) => {
      const [ga, ta] = rank(a);
      const [gb, tb] = rank(b);
      return ga - gb || ta - tb;
    });
}

function Marker({ children }: { children: ReactNode }) {
  return (
    <Typography
      variant="h2"
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 1,
        pt: 2,
        pb: 0.5,
        color: "blue.main",
        fontWeight: 700,
        fontSize: "0.8125rem",
        "&::after": {
          content: '""',
          flex: 1,
          borderTop: 1,
          borderColor: "blue.main",
        },
      }}
    >
      {children}
    </Typography>
  );
}

function List({
  items,
  now,
  empty,
}: {
  items: Item[];
  now: Date;
  empty?: string | undefined;
}) {
  if (items.length === 0) {
    return empty === undefined ? null : (
      <Typography color="text.secondary" sx={{ py: 2 }}>
        {empty}
      </Typography>
    );
  }
  return (
    <Box>
      {items.map((it) => (
        <ItemRow key={it.panelId} item={it} now={now} />
      ))}
    </Box>
  );
}
