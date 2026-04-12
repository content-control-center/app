/**
 * Session API — login / logout.
 *
 * The backend sets an HTTPOnly session cookie on success, so the frontend
 * never handles the token directly. We only care whether the request
 * succeeded and, for login, what the response body tells us about the
 * authenticated user.
 */

import type { LoginPayload, Session } from "@/types/session";

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
  }
  return sessionCached;
}

export function invalidateSession(): void {
  sessionCached = null;
}

async function fetchSession(): Promise<boolean> {
  try {
    const res = await fetch("/api/users/", {
      method: "GET",
      credentials: "include",
    });
    return res.ok;
  } catch {
    return false;
  }
}

async function errorMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (typeof body.error === "string" && body.error.length > 0) return body.error;
  } catch {
    // fall through
  }
  return fallback;
}
