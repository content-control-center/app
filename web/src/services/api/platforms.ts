import type { Platform } from "@/types/campaigns";

const BASE = "/api/platforms";

export async function listPlatforms(): Promise<Platform[]> {
  const res = await fetch(BASE, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch platforms"));
  }
  return (await res.json()) as Platform[];
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
