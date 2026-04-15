import type {
  Campaign,
  CampaignType,
  CreateCampaignPayload,
  UpdateCampaignPayload,
} from "@/types/campaigns";

const BASE = "/api/campaigns";
const TYPES_BASE = "/api/campaign_types";

export async function listCampaigns(): Promise<Campaign[]> {
  const res = await fetch(BASE, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch campaigns"));
  }
  return (await res.json()) as Campaign[];
}

export async function getCampaign(id: string): Promise<Campaign> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch campaign"));
  }
  return (await res.json()) as Campaign;
}

export async function createCampaign(payload: CreateCampaignPayload): Promise<Campaign> {
  const res = await fetch(BASE, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to create campaign"));
  }
  return (await res.json()) as Campaign;
}

export async function updateCampaign(id: string, payload: UpdateCampaignPayload): Promise<Campaign> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to update campaign"));
  }
  return (await res.json()) as Campaign;
}

export async function deleteCampaign(id: string): Promise<void> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to delete campaign"));
  }
}

export async function listCampaignTypes(): Promise<CampaignType[]> {
  const res = await fetch(TYPES_BASE, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch campaign types"));
  }
  return (await res.json()) as CampaignType[];
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
