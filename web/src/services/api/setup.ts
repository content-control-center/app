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

async function fetchSetupComplete(): Promise<boolean> {
  try {
    const res = await fetch("/api/settings/setup_complete", {
      method: "GET",
      credentials: "include",
    });
    if (!res.ok) return false;
    const parsed = settingResponseSchema.safeParse(await res.json());
    if (!parsed.success) return false;
    return parsed.data.value === "true";
  } catch {
    return false;
  }
}
