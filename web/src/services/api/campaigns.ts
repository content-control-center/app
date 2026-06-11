import type {
  Campaign,
  CampaignType,
  CreateCampaignPayload,
  UpdateCampaignPayload,
} from "@/types/campaigns";
import { apiJson, apiVoid } from "./http";

const BASE = "/api/campaigns";
const TYPES_BASE = "/api/campaign_types";

export function listCampaigns(): Promise<Campaign[]> {
  return apiJson<Campaign[]>(BASE, "Unable to fetch campaigns");
}

export function getCampaign(id: string): Promise<Campaign> {
  return apiJson<Campaign>(`${BASE}/${id}`, "Unable to fetch campaign");
}

export function createCampaign(payload: CreateCampaignPayload): Promise<Campaign> {
  return apiJson<Campaign>(BASE, "Unable to create campaign", { method: "POST", body: payload });
}

export function updateCampaign(id: string, payload: UpdateCampaignPayload): Promise<Campaign> {
  return apiJson<Campaign>(`${BASE}/${id}`, "Unable to update campaign", { method: "PUT", body: payload });
}

export function deleteCampaign(id: string): Promise<void> {
  return apiVoid(`${BASE}/${id}`, "Unable to delete campaign", { method: "DELETE" });
}

export function listCampaignTypes(): Promise<CampaignType[]> {
  return apiJson<CampaignType[]>(TYPES_BASE, "Unable to fetch campaign types");
}
