import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteSecret, listSecrets, putSecret } from "@/services/api/secrets";
import type { SecretName } from "@/types/integrations";
import { PLATFORMS_KEY } from "@/hooks/usePlatforms";
import { ZERNIO_HEALTH_KEY } from "@/hooks/useZernio";

export const SECRETS_KEY = ["secrets"] as const;

export function useSecretsList() {
  return useQuery({
    queryKey: SECRETS_KEY,
    queryFn: listSecrets,
    staleTime: 30_000,
  });
}

export function useUpsertSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, value }: { name: SecretName; value: string }) => putSecret(name, value),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: SECRETS_KEY });
      if (vars.name === "zernio_api_key") {
        qc.invalidateQueries({ queryKey: ZERNIO_HEALTH_KEY });
        qc.invalidateQueries({ queryKey: PLATFORMS_KEY });
      }
    },
  });
}

export function useDeleteSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: SecretName) => deleteSecret(name),
    onSuccess: (_data, name) => {
      qc.invalidateQueries({ queryKey: SECRETS_KEY });
      if (name === "zernio_api_key") {
        qc.invalidateQueries({ queryKey: ZERNIO_HEALTH_KEY });
        qc.invalidateQueries({ queryKey: PLATFORMS_KEY });
      }
    },
  });
}
