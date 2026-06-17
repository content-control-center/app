/**
 * Session API — login / logout.
 *
 * The backend sets an HTTPOnly session cookie on success, so the frontend
 * never handles the token directly. We only care whether the request
 * succeeded and, for login, what the response body tells us about the
 * authenticated user.
 */

import type { LoginPayload, Session } from "@/types/session";
import { ServerUnavailableError, errorMessage, fetchOrThrowUnavailable } from "./errors";

export async function login(payload: LoginPayload): Promise<Session> {
  const res = await fetch("/api/sessions", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to log in"));
  }
  return (await res.json()) as Session;
}

export async function logout(): Promise<void> {
  const res = await fetch("/api/sessions", {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to log out"));
  }
}

let sessionCached: Promise<boolean> | null = null;

export function checkSession(): Promise<boolean> {
  if (sessionCached === null) {
    sessionCached = fetchSession();
    // Never cache a server-unreachable failure — a later retry must re-probe.
    void sessionCached.catch(() => {
      sessionCached = null;
    });
  }
  return sessionCached;
}

export function invalidateSession(): void {
  sessionCached = null;
}

async function fetchSession(): Promise<boolean> {
  // A network rejection surfaces as `ServerUnavailableError` (server down)
  // rather than being flattened into "not authenticated".
  const res = await fetchOrThrowUnavailable("/api/users/", {
    method: "GET",
    credentials: "include",
  });
  // A 5xx (incl. the dev proxy's 500 when the backend is down) is an outage,
  // not a "logged out" signal — let it surface as ServerUnavailableError.
  if (res.status >= 500) throw new ServerUnavailableError();
  return res.ok;
}
