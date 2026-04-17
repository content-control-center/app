import { z } from "zod";

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
  try {
    const res = await fetch("/api/settings/setup_complete", {
      method: "GET",
      credentials: "include",
    });
    // Backend leaves this endpoint open only while setup is incomplete; once
    // setup_complete=true it requires auth and returns 401 to anonymous callers.
    if (res.status === 401) return true;
    if (!res.ok) return false;
    const parsed = settingResponseSchema.safeParse(await res.json());
    if (!parsed.success) return false;
    return parsed.data.value === "true";
  } catch {
    return false;
  }
}
