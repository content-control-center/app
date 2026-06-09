import { z } from "zod";

import { ServerUnavailableError, fetchOrThrowUnavailable } from "./errors";

/**
 * Checks whether first-run setup has been completed. Used by the root route
 * guard to decide whether to send the user to `/setup`.
 *
 * The result is cached at module scope for the lifetime of the SPA so we only
 * hit the backend once. Concurrent callers share the in-flight promise.
 * Call `invalidateSetupComplete()` after a successful setup submission to
 * force the next call to refetch.
 */
const settingResponseSchema = z.object({
  key: z.string(),
  value: z.string(),
});

let cached: Promise<boolean> | null = null;

export function isSetupComplete(): Promise<boolean> {
  if (cached === null) {
    cached = fetchSetupComplete();
    // Never cache a server-unreachable failure — a later retry must re-probe.
    void cached.catch(() => {
      cached = null;
    });
  }
  return cached;
}

export function invalidateSetupComplete(): void {
  cached = null;
}

export async function markSetupComplete(): Promise<void> {
  const res = await fetch("/api/settings/setup_complete", {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value: "true" }),
  });
  if (!res.ok) {
    throw new Error("Failed to mark setup as complete");
  }
}

async function fetchSetupComplete(): Promise<boolean> {
  // A network rejection here means the server is unreachable; it surfaces as a
  // `ServerUnavailableError` so the caller can distinguish it from a real
  // response. Everything below has a live server answering.
  const res = await fetchOrThrowUnavailable("/api/settings/setup_complete", {
    method: "GET",
    credentials: "include",
  });
  // Backend leaves this endpoint open only while setup is incomplete; once
  // setup_complete=true it requires auth and returns 401 to anonymous callers.
  if (res.status === 401) return true;
  // A 5xx means the backend — or a proxy in front of it (the Vite dev proxy
  // returns 500, nginx/traefik return 502/503/504) — is down or broken, NOT
  // that setup is incomplete. A healthy fresh install answers 200 here. Treat
  // it as an outage so we don't misroute the user into the setup wizard.
  if (res.status >= 500) throw new ServerUnavailableError();
  if (!res.ok) return false;
  try {
    const parsed = settingResponseSchema.safeParse(await res.json());
    return parsed.success && parsed.data.value === "true";
  } catch {
    return false;
  }
}
