import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createConnectLink,
  getZernioHealth,
  listZernioAccounts,
  triggerZernioSync,
} from "@/services/api/zernio";
import { PLATFORMS_KEY } from "@/hooks/usePlatforms";

export const ZERNIO_HEALTH_KEY = ["zernio", "health"] as const;
export const ZERNIO_ACCOUNTS_KEY = ["zernio", "accounts"] as const;

export function useZernioHealth() {
  return useQuery({
    queryKey: ZERNIO_HEALTH_KEY,
    queryFn: getZernioHealth,
    staleTime: 60_000,
    // Only poll while the integration is reconnecting; otherwise rely on
    // explicit invalidation (after upserting the Zernio key, after Sync now).
    refetchInterval: (query) =>
      query.state.data?.state === "degraded" ? 60_000 : false,
  });
}

export function useZernioAccounts() {
  return useQuery({
    queryKey: ZERNIO_ACCOUNTS_KEY,
    queryFn: listZernioAccounts,
    staleTime: 30_000,
  });
}

export function useCreateConnectLink() {
  return useMutation({
    mutationFn: (platform: string) => createConnectLink(platform),
  });
}

export function useTriggerZernioSync() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => triggerZernioSync(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ZERNIO_ACCOUNTS_KEY });
      qc.invalidateQueries({ queryKey: ZERNIO_HEALTH_KEY });
      qc.invalidateQueries({ queryKey: PLATFORMS_KEY });
    },
  });
}
