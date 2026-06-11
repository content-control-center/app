import type { AssetStatus } from "@/types/content";
import type { StatusTone } from "@/components/ui/status-badge";

export type UploadKind = "md" | "pdf";

const MB = 1 << 20;

/** Per-extension upload limits, mirroring the Go backend (assets.go). */
export const UPLOAD_LIMITS: Record<UploadKind, number> = {
  md: 10 * MB,
  pdf: 50 * MB,
};

export const UPLOAD_ACCEPT = ".md,.pdf";

/** Human-readable summary of the limits, shown in the upload modal. */
export const UPLOAD_LIMITS_LABEL =
  "Markdown (.md) up to 10 MB · PDF (.pdf) up to 50 MB";

/** Maps a filename extension to an upload kind, or null if unsupported. */
export function detectUploadKind(filename: string): UploadKind | null {
  const ext = filename.slice(filename.lastIndexOf(".")).toLowerCase();
  if (ext === ".md") return "md";
  if (ext === ".pdf") return "pdf";
  return null;
}

export type UploadValidation =
  | { ok: true; kind: UploadKind }
  | { ok: false; error: string };

/**
 * Client-side guard that mirrors the backend's accepted types and size caps so
 * obviously-bad files fail instantly without a network round-trip. The error
 * strings match the messages the server returns for the same conditions.
 */
export function validateUploadFile(file: File): UploadValidation {
  const kind = detectUploadKind(file.name);
  if (!kind) {
    return { ok: false, error: "only .md and .pdf files are accepted" };
  }
  const limit = UPLOAD_LIMITS[kind];
  if (file.size > limit) {
    return {
      ok: false,
      error: `file exceeds maximum size of ${Math.round(limit / MB)} MB`,
    };
  }
  return { ok: true, kind };
}

/** Async statuses that will never change again. */
export function isTerminalStatus(status: AssetStatus): boolean {
  return status === "ready" || status === "partial" || status === "failed";
}

const STATUS_BADGE: Record<AssetStatus, { tone: StatusTone; label: string }> = {
  pending: { tone: "progress", label: "Pending" },
  processing: { tone: "progress", label: "Processing" },
  ready: { tone: "positive", label: "Ready" },
  partial: { tone: "warn", label: "Partial" },
  failed: { tone: "destructive", label: "Failed" },
};

/** Tone + label for rendering an asset status as a StatusBadge. */
export function statusToBadge(status: AssetStatus): {
  tone: StatusTone;
  label: string;
} {
  return STATUS_BADGE[status] ?? { tone: "neutral", label: status };
}

/** Compact human-readable file size, e.g. "1.4 MB". */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${Math.round(kb)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}
