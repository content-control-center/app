import { create } from "zustand";
import { devtools } from "zustand/middleware";
import { uploadAssetFile } from "@/services/api/uploads";
import { validateUploadFile, type UploadKind } from "@/lib/assetStatus";
import type { AssetStatus } from "@/types/content";

/**
 * A tracked upload moves: uploading (HTTP transfer) → processing (async backend
 * work, watched by polling) → ready | partial | failed. Validation failures and
 * network errors land directly in `failed`.
 */
export type UploadPhase =
  | "uploading"
  | "processing"
  | "ready"
  | "partial"
  | "failed";

export type UploadItem = {
  id: string;
  file: File;
  filename: string;
  sizeBytes: number;
  kind: UploadKind | null;
  phase: UploadPhase;
  progress: number; // 0–100, meaningful while `uploading`
  assetId?: string;
  error?: string;
};

type UploadState = {
  items: UploadItem[];
  /** Validate and begin uploading each file; invalid files appear as failed. */
  enqueue: (files: File[] | FileList) => void;
  /** Re-run a failed upload from its original File. */
  retry: (id: string) => void;
  /** Sync a polled backend asset status into the tracked item. */
  setStatus: (id: string, status: AssetStatus) => void;
  /** Drop a single item from the tracker. */
  remove: (id: string) => void;
  /** Drop every item that has reached a terminal phase. */
  clearFinished: () => void;
};

const newId = () => crypto.randomUUID();

function statusToPhase(status: AssetStatus): UploadPhase {
  if (status === "ready" || status === "partial" || status === "failed") {
    return status;
  }
  return "processing";
}

export const useUploadStore = create<UploadState>()(
  devtools(
    (set, get) => {
      const patch = (id: string, changes: Partial<UploadItem>) =>
        set((state) => ({
          items: state.items.map((it) =>
            it.id === id ? { ...it, ...changes } : it,
          ),
        }));

      const start = (id: string, file: File) => {
        patch(id, { phase: "uploading", progress: 0, error: undefined });
        uploadAssetFile(file, { onProgress: (p) => patch(id, { progress: p }) })
          .then((result) => {
            if (result.status === "failed") {
              patch(id, {
                phase: "failed",
                error: result.error ?? "Upload failed",
              });
              return;
            }
            patch(id, {
              assetId: result.asset_id,
              progress: 100,
              phase: result.asset
                ? statusToPhase(result.asset.status)
                : "processing",
            });
          })
          .catch((err: unknown) => {
            patch(id, {
              phase: "failed",
              error: err instanceof Error ? err.message : "Upload failed",
            });
          });
      };

      return {
        items: [],

        enqueue: (files) => {
          const items: UploadItem[] = Array.from(files).map((file) => {
            const validation = validateUploadFile(file);
            const base = {
              id: newId(),
              file,
              filename: file.name,
              sizeBytes: file.size,
              progress: 0,
            };
            return validation.ok
              ? { ...base, kind: validation.kind, phase: "uploading" as const }
              : {
                  ...base,
                  kind: null,
                  phase: "failed" as const,
                  error: validation.error,
                };
          });
          if (items.length === 0) return;

          set((state) => ({ items: [...state.items, ...items] }));
          for (const item of items) {
            if (item.phase === "uploading") start(item.id, item.file);
          }
        },

        retry: (id) => {
          const item = get().items.find((it) => it.id === id);
          if (!item) return;
          const validation = validateUploadFile(item.file);
          if (!validation.ok) {
            patch(id, { phase: "failed", error: validation.error });
            return;
          }
          start(id, item.file);
        },

        setStatus: (id, status) => patch(id, { phase: statusToPhase(status) }),

        remove: (id) =>
          set((state) => ({ items: state.items.filter((it) => it.id !== id) })),

        clearFinished: () =>
          set((state) => ({
            items: state.items.filter(
              (it) => it.phase === "uploading" || it.phase === "processing",
            ),
          })),
      };
    },
    { name: "upload-store" },
  ),
);
