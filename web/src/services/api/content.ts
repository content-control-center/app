import type { Asset, CreateAssetPayload, UpdateAssetPayload } from "@/types/content";

const BASE = "/api/content-bank/assets";

export async function listAssets(): Promise<Asset[]> {
  const res = await fetch(BASE, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch assets"));
  }
  return (await res.json()) as Asset[];
}

export async function getAsset(id: string): Promise<Asset> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch asset"));
  }
  return (await res.json()) as Asset;
}

export async function createAsset(payload: CreateAssetPayload): Promise<Asset> {
  const res = await fetch(BASE, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to create asset"));
  }
  return (await res.json()) as Asset;
}

export async function updateAsset(id: string, payload: UpdateAssetPayload): Promise<Asset> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to update asset"));
  }
  return (await res.json()) as Asset;
}

export async function deleteAsset(id: string): Promise<void> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to delete asset"));
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
