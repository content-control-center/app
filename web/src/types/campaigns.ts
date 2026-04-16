import type { Tag } from "@/types/content";

export type CampaignStatus = "draft" | "active";

export type CampaignPlatform = {
  id: string;
  post_types: string[];
};

export type Platform = {
  id: string;
  name: string;
  post_types: Record<string, string>;
  cadence: string;
  constraints: string;
  created_at: string;
  updated_at: string;
};

export type CampaignTypePhase = {
  id: string;
  campaign_type_id: string;
  name: string;
  purpose: string;
  sequence: number;
};

export type CampaignType = {
  id: string;
  name: string;
  label: string;
  description: string;
  is_system: boolean;
  phases?: CampaignTypePhase[];
};

export type Campaign = {
  id: string;
  name: string;
  description: string;
  target_persona: string;
  key_messages: string;
  tone_guidelines: string;
  use_assets: boolean;
  asset_ids: string[];
  target_platforms: CampaignPlatform[];
  campaign_type_id: string;
  status: CampaignStatus;
  start_date: string | null;
  end_date: string | null;
  estimated_post_count: number | null;
  language: string;
  budget: number | null;
  currency: string;
  tag_ids: string[];
  tags: Tag[];
  platforms: Platform[];
  campaign_type?: CampaignType | null;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type CreateCampaignPayload = {
  name: string;
  campaign_type_id: string;
  description?: string;
  target_persona?: string;
  key_messages?: string;
  tone_guidelines?: string;
  use_assets?: boolean;
  asset_ids?: string[];
  target_platforms?: CampaignPlatform[];
  status?: CampaignStatus;
  start_date?: string | null;
  end_date?: string | null;
  estimated_post_count?: number | null;
  budget?: number | null;
  currency?: string;
  language?: string;
  tag_ids?: string[];
};

export type UpdateCampaignPayload = CreateCampaignPayload;
