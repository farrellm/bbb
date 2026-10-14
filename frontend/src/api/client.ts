import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// Typed client for the Go API; the types are generated from
// backend/api/openapi.yaml (npm run gen:api).
export const api = createClient<paths>({ baseUrl: "/api" });

type Schemas = components["schemas"];
export type User = Schemas["User"];
export type Item = Schemas["Item"];
export type ItemEvent = Schemas["Event"];
export type TimelineEvent = Schemas["TimelineEvent"];
export type Division = Schemas["Division"];
export type EventKind = Schemas["EventKind"];
export type SignupLink = Schemas["SignupLink"];
export type Invite = Schemas["Invite"];
export type Preferences = Schemas["Preferences"];
export type Device = Schemas["Device"];

class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

// data returns a successful response's body, or throws an ApiError with
// the server's message.
export async function data<T>(
  call: Promise<{
    data?: T;
    error?: { error: string } | undefined;
    response: Response;
  }>,
): Promise<T> {
  const { data: body, error, response } = await call;
  if (!response.ok || body === undefined) {
    throw new ApiError(
      response.status,
      error?.error ?? `${response.status} ${response.statusText}`,
    );
  }
  return body;
}

// ok waits for a call answered with no body (a 204), throwing an ApiError
// with the server's message if it failed.
export async function ok(
  call: Promise<{ error?: { error: string } | undefined; response: Response }>,
): Promise<void> {
  const { error, response } = await call;
  if (!response.ok) {
    throw new ApiError(
      response.status,
      error?.error ?? `${response.status} ${response.statusText}`,
    );
  }
}

// markRead records events as read. Failures are only logged: the worst
// case is a mark that shows again next time.
export function markRead(ids: number[]): void {
  if (ids.length === 0) return;
  api
    .POST("/events/read", { body: { ids } })
    .catch((err: unknown) => console.warn("mark read", err));
}
