import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getAsset } from "@/services/api/content";
import { Button } from "@/components/ui/button";
import { ArrowsLeftRight, X } from "@phosphor-icons/react";
import { StatusBadge, type StatusTone } from "@/components/ui/status-badge";
import { isTerminalStatus } from "@/lib/assetStatus";
import { useUploadStore, type UploadItem } from "@/stores/uploadStore";

const PHASE_BADGE: Record<
  UploadItem["phase"],
  { tone: StatusTone; label: string }
> = {
  uploading: { tone: "progress", label: "Uploading" },
  processing: { tone: "progress", label: "Processing" },
  ready: { tone: "positive", label: "Ready" },
  partial: { tone: "warn", label: "Partial" },
  failed: { tone: "destructive", label: "Failed" },
};

const POLL_MS = 2000;

/**
 * One line in the upload tracker. While an asset is processing it polls
 * GET /assets/:id until the status is terminal, syncing each result back into
 * the store and refreshing the asset list once it settles.
 */
export function UploadRow({ item }: { item: UploadItem }) {
  const setStatus = useUploadStore((s) => s.setStatus);
  const remove = useUploadStore((s) => s.remove);
  const retry = useUploadStore((s) => s.retry);
  const qc = useQueryClient();

  const shouldPoll = item.phase === "processing" && !!item.assetId;

  const { data: asset } = useQuery({
    queryKey: ["assets", item.assetId],
    queryFn: () => getAsset(item.assetId as string),
    enabled: shouldPoll,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && isTerminalStatus(status) ? false : POLL_MS;
    },
  });

  // Mirror polled status into the tracker and refresh the list when it settles.
  useEffect(() => {
    if (!asset) return;
    setStatus(item.id, asset.status);
    if (isTerminalStatus(asset.status)) {
      qc.invalidateQueries({ queryKey: ["assets"] });
    }
  }, [asset, item.id, setStatus, qc]);

  const badge =
    item.phase === "uploading"
      ? { tone: "progress" as const, label: `Uploading ${item.progress}%` }
      : PHASE_BADGE[item.phase];

  return (
    <div className="flex flex-col gap-1 py-2">
      <div className="flex items-center justify-between gap-3">
        <span className="truncate text-sm text-foreground">
          {item.filename}
        </span>
        <div className="flex items-center gap-1 shrink-0">
          <StatusBadge tone={badge.tone} label={badge.label} />
          {item.phase === "failed" && (
            <Button
              variant="ghost"
              size="xsIcon"
              onClick={() => retry(item.id)}
              aria-label="Retry upload"
            >
              <ArrowsLeftRight className="size-4" />
            </Button>
          )}
          <Button
            variant="ghost"
            size="xsIcon"
            onClick={() => remove(item.id)}
            aria-label="Dismiss"
          >
            <X className="size-4" />
          </Button>
        </div>
      </div>

      {item.phase === "uploading" && (
        <div className="h-0.5 w-full bg-quaternary">
          <div
            className="h-full bg-chart-4 transition-all"
            style={{ width: `${item.progress}%` }}
          />
        </div>
      )}

      {item.phase === "failed" && item.error && (
        <span className="text-xs text-destructive">{item.error}</span>
      )}
    </div>
  );
}
