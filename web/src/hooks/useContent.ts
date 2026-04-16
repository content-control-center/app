import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  listAssets,
  getAsset,
  createAsset,
  updateAsset,
  deleteAsset,
} from "@/services/api/content";
import type { CreateAssetPayload, UpdateAssetPayload } from "@/types/content";

const ASSETS_KEY = ["assets"] as const;
const assetKey = (id: string) => ["assets", id] as const;

export function useAssets() {
  return useQuery({
    queryKey: ASSETS_KEY,
    queryFn: listAssets,
  });
}

export function useAsset(id: string) {
  return useQuery({
    queryKey: assetKey(id),
    queryFn: () => getAsset(id),
    enabled: !!id,
  });
}

export function useCreateAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateAssetPayload) => createAsset(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ASSETS_KEY });
    },
  });
}

export function useUpdateAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateAssetPayload }) =>
      updateAsset(id, payload),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: ASSETS_KEY });
      qc.invalidateQueries({ queryKey: assetKey(id) });
    },
  });
}

export function useDeleteAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteAsset(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ASSETS_KEY });
    },
  });
}
