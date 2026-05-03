import {
  ZernioError,
  type ConnectLinkResponse,
  type ZernioAccountsResponse,
  type ZernioErrorCode,
  type ZernioHealth,
} from "@/types/integrations";

const BASE = "/api/integrations/zernio";

export async function getZernioHealth(): Promise<ZernioHealth> {
  const res = await fetch(`${BASE}/health`, { method: "GET", credentials: "include" });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch Zernio health"));
  }
  return (await res.json()) as ZernioHealth;
}

export async function listZernioAccounts(): Promise<ZernioAccountsResponse> {
  const res = await fetch(`${BASE}/accounts`, { method: "GET", credentials: "include" });
  if (!res.ok) {
    throw await zernioError(res, "Unable to fetch Zernio accounts");
  }
  return (await res.json()) as ZernioAccountsResponse;
}

export async function createConnectLink(platform: string): Promise<ConnectLinkResponse> {
  const res = await fetch(`${BASE}/connect-links`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ platform }),
  });
  if (!res.ok) {
    throw await zernioError(res, "Unable to create connect link");
  }
  return (await res.json()) as ConnectLinkResponse;
}

export async function triggerZernioSync(): Promise<void> {
  const res = await fetch(`${BASE}/sync`, { method: "POST", credentials: "include" });
  if (!res.ok && res.status !== 202) {
    throw await zernioError(res, "Unable to trigger sync");
  }
}

const KNOWN_CODES: ReadonlySet<ZernioErrorCode> = new Set<ZernioErrorCode>([
  "integration_disabled",
  "integration_degraded",
  "rate_limited",
  "invalid_platform",
]);

async function zernioError(res: Response, fallback: string): Promise<ZernioError> {
  const msg = await errorMessage(res, fallback);
  const code: ZernioErrorCode = KNOWN_CODES.has(msg as ZernioErrorCode)
    ? (msg as ZernioErrorCode)
    : "unknown";
  let retryAfterSeconds: number | undefined;
  const retryAfter = res.headers.get("Retry-After");
  if (retryAfter) {
    const n = parseInt(retryAfter, 10);
    if (Number.isFinite(n) && n > 0) retryAfterSeconds = n;
  }
  return new ZernioError(code, res.status, msg, retryAfterSeconds);
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
