import { clientsClaim } from "workbox-core";
import {
  cleanupOutdatedCaches,
  createHandlerBoundToURL,
  precacheAndRoute,
  type PrecacheEntry,
} from "workbox-precaching";
import createClient from "openapi-fetch";
import { NavigationRoute, registerRoute } from "workbox-routing";
import type { components, paths } from "./api/schema";
import { decodeKey, subscriptionBody } from "./lib/pushKeys";

declare const self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: (PrecacheEntry | string)[];
};

type PushPayload = components["schemas"]["PushPayload"];

// The worker's own API client; it shares the app's cookie.
const api = createClient<paths>({ baseUrl: "/api" });

// ── App shell ────────────────────────────────────────────────────────────
// A new worker takes over at once (registerType autoUpdate), and the app
// shell is served from the precache so the home-screen app opens offline.

void self.skipWaiting();
clientsClaim();
cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);
if (import.meta.env.PROD) {
  // Client-side routes get index.html; /api (including the sign-in
  // redirects, which are navigations) always goes to the server.
  registerRoute(
    new NavigationRoute(createHandlerBoundToURL("index.html"), {
      denylist: [/^\/api\//],
    }),
  );
}

// ── Push ─────────────────────────────────────────────────────────────────

// iOS revokes the subscription of a worker that receives a push without
// showing a notification, so even an unreadable push shows something.
const fallback: PushPayload = {
  title: "BBB Watch",
  body: "Something changed on the Big Blue Bulletin.",
  url: "/",
  tag: "fallback",
  badge: 0,
};

function readPayload(data: PushMessageData | null): PushPayload {
  let p: unknown;
  try {
    p = data?.json();
  } catch {
    return fallback;
  }
  if (typeof p !== "object" || p === null) return fallback;
  const str = (k: keyof PushPayload, def: string): string => {
    const v: unknown = Reflect.get(p, k);
    return typeof v === "string" ? v : def;
  };
  const badge: unknown = Reflect.get(p, "badge");
  return {
    title: str("title", fallback.title),
    body: str("body", fallback.body),
    url: str("url", fallback.url),
    tag: str("tag", fallback.tag),
    badge: typeof badge === "number" ? badge : 0,
  };
}

async function show(p: PushPayload): Promise<void> {
  await self.registration.showNotification(p.title, {
    body: p.body,
    tag: p.tag,
    data: { url: p.url },
    icon: "/icon-192.png",
  });
  if ("setAppBadge" in self.navigator) {
    await self.navigator.setAppBadge(p.badge).catch(() => undefined);
  }
}

self.addEventListener("push", (event) => {
  event.waitUntil(show(readPayload(event.data)));
});

// A tap opens the notification's page in the open app if there is one,
// else in a new window.
async function open(path: string): Promise<void> {
  const url = new URL(path, self.location.origin);
  if (url.origin !== self.location.origin) url.href = self.location.origin;
  const [win] = await self.clients.matchAll({
    type: "window",
    includeUncontrolled: true,
  });
  if (win !== undefined) {
    try {
      const focused = await win.focus();
      await focused.navigate(url.href);
      return;
    } catch {
      // Not focusable, or not ours to navigate: open a window instead.
    }
  }
  await self.clients.openWindow(url.href);
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const data: unknown = event.notification.data;
  const url =
    typeof data === "object" &&
    data !== null &&
    "url" in data &&
    typeof data.url === "string"
      ? data.url
      : "/";
  event.waitUntil(open(url));
});

// When the browser replaces a subscription (keys expire or rotate), save
// the new one, subscribing again if the browser didn't.
type SubscriptionChangeEvent = ExtendableEvent & {
  newSubscription?: PushSubscription | null;
};

async function resubscribe(event: SubscriptionChangeEvent): Promise<void> {
  let sub = event.newSubscription ?? null;
  if (sub === null) {
    const { data } = await api.GET("/push/vapid-public-key");
    if (data === undefined) return;
    sub = await self.registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: decodeKey(data.key),
    });
  }
  await api.POST("/push/subscribe", { body: subscriptionBody(sub) });
}

function isSubscriptionChange(e: Event): e is SubscriptionChangeEvent {
  return "waitUntil" in e;
}

self.addEventListener("pushsubscriptionchange", (event: Event) => {
  if (isSubscriptionChange(event)) event.waitUntil(resubscribe(event));
});
