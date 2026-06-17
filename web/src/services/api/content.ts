import type { Asset, CreateAssetPayload, UpdateAssetPayload } from "@/types/content";
import { apiJson, apiVoid } from "./http";

const BASE = "/api/content-bank/assets";

export function listAssets(): Promise<Asset[]> {
  return apiJson<Asset[]>(BASE, "Unable to fetch assets");
}

export function getAsset(id: string): Promise<Asset> {
  return apiJson<Asset>(`${BASE}/${id}`, "Unable to fetch asset");
}

export function createAsset(payload: CreateAssetPayload): Promise<Asset> {
  return apiJson<Asset>(BASE, "Unable to create asset", { method: "POST", body: payload });
}

export function updateAsset(id: string, payload: UpdateAssetPayload): Promise<Asset> {
  return apiJson<Asset>(`${BASE}/${id}`, "Unable to update asset", { method: "PUT", body: payload });
}

export function deleteAsset(id: string): Promise<void> {
  return apiVoid(`${BASE}/${id}`, "Unable to delete asset", { method: "DELETE" });
}
