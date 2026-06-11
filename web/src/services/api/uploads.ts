import type { Asset } from "@/types/content";

const UPLOAD_URL = "/api/content-bank/assets/upload";

/** Per-file outcome returned by the backend's batch upload endpoint. */
export type UploadResult = {
  filename: string;
  asset_id?: string;
  status: "created" | "failed";
  error?: string;
  asset?: Asset;
};

type UploadOptions = {
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
};

/**
 * Uploads a single file to the content bank. Uses XMLHttpRequest (not fetch) so
 * the transfer progress can be reported per file via `onProgress`. The endpoint
 * accepts a batch under the `files` field and always responds 201 with a
 * `results` array; we send one file per request, so we return `results[0]`.
 */
export function uploadAssetFile(
  file: File,
  opts: UploadOptions = {},
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.append("files", file);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", UPLOAD_URL, true);
    xhr.withCredentials = true;
    xhr.responseType = "json";

    if (opts.onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          opts.onProgress?.(Math.round((e.loaded / e.total) * 100));
        }
      };
    }

    xhr.onload = () => {
      const body = xhr.response as
        | { results?: UploadResult[]; error?: string }
        | null;
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new Error(extractError(body, "Upload failed")));
        return;
      }
      const result = body?.results?.[0];
      if (!result) {
        reject(new Error("Server returned no upload result"));
        return;
      }
      resolve(result);
    };

    xhr.onerror = () => reject(new Error("Network error during upload"));
    xhr.onabort = () =>
      reject(new DOMException("Upload aborted", "AbortError"));

    if (opts.signal) {
      if (opts.signal.aborted) {
        xhr.abort();
        return;
      }
      opts.signal.addEventListener("abort", () => xhr.abort(), { once: true });
    }

    xhr.send(form);
  });
}

function extractError(body: unknown, fallback: string): string {
  if (body && typeof body === "object" && "error" in body) {
    const e = (body as { error?: unknown }).error;
    if (typeof e === "string" && e.length > 0) return e;
  }
  return fallback;
}
