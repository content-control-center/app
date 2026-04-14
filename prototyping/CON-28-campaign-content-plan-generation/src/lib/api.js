export const initialCreateForm = {
  name: "Importance of Modern Haskell in AI Development",
  description:
    "Show AI product teams and research engineers why modern Haskell is practical for building reliable model-serving pipelines, interpretable tooling, and formally safer data transformations.",
  target_persona:
    "Senior ML engineers, platform engineers, and technical founders evaluating strongly typed languages for production AI systems.",
  key_messages:
    "Modern Haskell enables correctness-by-construction, expressive abstractions for model workflows, and maintainable AI infrastructure with fewer runtime surprises.",
  tone_guidelines:
    "Clear, technically grounded, and pragmatic. Emphasize engineering trade-offs, concrete examples, and practical adoption paths.",
  use_pieces: false,
  pieces_ids: "",
  campaign_type_id: "",
  status: "draft",
  start_date: "2026-05-01",
  end_date: "2026-05-31",
  estimated_post_count: "10",
  budget: "1500",
  currency: "USD",
  language: "en",
  tag_ids: "",
};

export const initialDraftForm = {
  baseUrl: "",
  campaignId: "",
};

export function normalizeList(value) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function parseOptionalNumber(value, label) {
  if (value === "") return null;
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) throw new Error(`${label} must be a valid number`);
  return parsed;
}

export function toISOStringOrNull(dateValue) {
  if (!dateValue) return null;
  const parsed = new Date(dateValue);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed.toISOString();
}

export function buildGenerateDraftEndpoint(baseUrl, campaignId, encodeCampaignId = true) {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
  const normalizedCampaignId = campaignId.trim();
  const campaignIdSegment = encodeCampaignId
    ? encodeURIComponent(normalizedCampaignId)
    : normalizedCampaignId;
  const path = `/api/campaigns/${campaignIdSegment}/generate-draft`;
  return normalizedBaseUrl ? `${normalizedBaseUrl}${path}` : path;
}

export function buildCreateCampaignEndpoint(baseUrl) {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
  return normalizedBaseUrl ? `${normalizedBaseUrl}/api/campaigns` : "/api/campaigns";
}

export function buildCampaignTypesEndpoint(baseUrl) {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
  return normalizedBaseUrl ? `${normalizedBaseUrl}/api/campaign_types` : "/api/campaign_types";
}
