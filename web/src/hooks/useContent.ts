import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  listPieces,
  getPiece,
  createPiece,
  updatePiece,
  deletePiece,
} from "@/services/api/content";
import type { CreatePiecePayload, UpdatePiecePayload } from "@/types/content";

const PIECES_KEY = ["pieces"] as const;
const pieceKey = (id: string) => ["pieces", id] as const;

export function usePieces() {
  return useQuery({
    queryKey: PIECES_KEY,
    queryFn: listPieces,
  });
}

export function usePiece(id: string) {
  return useQuery({
    queryKey: pieceKey(id),
    queryFn: () => getPiece(id),
    enabled: !!id,
  });
}

export function useCreatePiece() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreatePiecePayload) => createPiece(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: PIECES_KEY });
    },
  });
}

export function useUpdatePiece() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdatePiecePayload }) =>
      updatePiece(id, payload),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: PIECES_KEY });
      qc.invalidateQueries({ queryKey: pieceKey(id) });
    },
  });
}

export function useDeletePiece() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deletePiece(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: PIECES_KEY });
    },
  });
}
