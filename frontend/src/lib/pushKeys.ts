import type { components } from "../api/schema";

// Helpers shared by the app and the service worker, so they use only what
// both have.

type SubscriptionIn = components["schemas"]["SubscriptionIn"];

// decodeKey turns the server's base64url VAPID key into the bytes
// PushManager.subscribe takes.
export function decodeKey(b64url: string): Uint8Array<ArrayBuffer> {
  const b64 = b64url.replaceAll("-", "+").replaceAll("_", "/");
  const raw = atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4));
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

// sameKey reports whether a subscription was made with this VAPID key. One
// made with an older key can't receive the server's pushes.
export function sameKey(
  subscribed: ArrayBuffer | null | undefined,
  b64url: string,
): boolean {
  if (subscribed === null || subscribed === undefined) return false;
  const a = new Uint8Array(subscribed);
  const b = decodeKey(b64url);
  return a.length === b.length && a.every((v, i) => v === b[i]);
}

// subscriptionBody is what POST /push/subscribe takes.
export function subscriptionBody(sub: PushSubscription): SubscriptionIn {
  const json = sub.toJSON();
  return {
    endpoint: sub.endpoint,
    keys: {
      p256dh: json.keys?.["p256dh"] ?? "",
      auth: json.keys?.["auth"] ?? "",
    },
  };
}
