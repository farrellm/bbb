import { api, data, ok } from "../api/client";
import { decodeKey, sameKey, subscriptionBody } from "./pushKeys";
import { isStandalone } from "./standalone";

// isIOS is true on iPhone and iPad, including iPads that report a Mac.
export function isIOS(): boolean {
  const ua = navigator.userAgent;
  return (
    /iPhone|iPad|iPod/.test(ua) ||
    (ua.includes("Macintosh") && navigator.maxTouchPoints > 1)
  );
}

export type PushSupport = "supported" | "needs-install" | "unsupported";

// pushSupport says whether this browser can get notifications. iOS only
// allows push in a home-screen app.
export function pushSupport(): PushSupport {
  if (isIOS() && !isStandalone()) return "needs-install";
  if (
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  )
    return "supported";
  return "unsupported";
}

export class PermissionDenied extends Error {
  constructor() {
    super("Notifications are blocked");
    this.name = "PermissionDenied";
  }
}

// currentSubscription is this browser's push subscription, if it has one.
export async function currentSubscription(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.ready;
  return reg.pushManager.getSubscription();
}

// enablePush asks for permission, subscribes this browser and saves the
// subscription for the signed-in user. Call it straight from a tap: iOS
// only shows the permission prompt in response to one.
export async function enablePush(): Promise<PushSubscription> {
  const permission = await Notification.requestPermission();
  if (permission !== "granted") throw new PermissionDenied();
  const { key } = await data(api.GET("/push/vapid-public-key"));
  const reg = await navigator.serviceWorker.ready;
  let sub = await reg.pushManager.getSubscription();
  if (sub !== null && !sameKey(sub.options.applicationServerKey, key)) {
    await sub.unsubscribe();
    sub = null;
  }
  sub ??= await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: decodeKey(key),
  });
  await data(api.POST("/push/subscribe", { body: subscriptionBody(sub) }));
  return sub;
}

// disablePush stops notifications to this browser, for the server and
// the browser both.
export async function disablePush(): Promise<void> {
  const sub = await currentSubscription();
  if (sub === null) return;
  await ok(api.DELETE("/push/subscribe", { body: { endpoint: sub.endpoint } }));
  await sub.unsubscribe();
}

// setBadge shows the unread count on the app icon where that's supported.
export function setBadge(unread: number): void {
  if (!("setAppBadge" in navigator)) return;
  const done =
    unread > 0 ? navigator.setAppBadge(unread) : navigator.clearAppBadge();
  done.catch(() => undefined);
}
