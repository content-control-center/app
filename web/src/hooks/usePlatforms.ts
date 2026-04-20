import { useQuery } from "@tanstack/react-query";
import { listPlatforms } from "@/services/api/platforms";

export const PLATFORMS_KEY = ["platforms"] as const;

export function usePlatforms() {
  return useQuery({
    queryKey: PLATFORMS_KEY,
    queryFn: listPlatforms,
    staleTime: Infinity,
  });
}
