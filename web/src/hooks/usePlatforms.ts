import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listPlatforms } from "@/services/api/platforms";
import { buildPlatformViews, type PlatformView } from "@/lib/platformDictionary";

export const PLATFORMS_KEY = ["platforms"] as const;

export function usePlatforms() {
  return useQuery({
    queryKey: PLATFORMS_KEY,
    queryFn: listPlatforms,
    staleTime: Infinity,
  });
}

export function usePlatformViews(): PlatformView[] {
  const { data } = usePlatforms();
  return useMemo(() => buildPlatformViews(data ?? []), [data]);
}
