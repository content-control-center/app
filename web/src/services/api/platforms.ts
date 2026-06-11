import type { Platform } from "@/types/campaigns";
import { apiJson } from "./http";

const BASE = "/api/platforms";

export function listPlatforms(): Promise<Platform[]> {
  return apiJson<Platform[]>(BASE, "Unable to fetch platforms");
}
