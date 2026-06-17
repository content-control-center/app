import { errorMessage } from "./errors";

const ALLOWED_IMAGE_MIMES = new Set([
  "image/jpeg",
  "image/png",
  "image/webp",
  "image/gif",
]);
const MAX_IMAGE_SIZE = 10 * 1024 * 1024;

export async function uploadImage(file: File): Promise<string> {
  if (!ALLOWED_IMAGE_MIMES.has(file.type)) {
    throw new Error(`Unsupported image type: ${file.type || "unknown"}`);
  }
  if (file.size > MAX_IMAGE_SIZE) {
    throw new Error(`Image exceeds maximum size of ${MAX_IMAGE_SIZE >> 20} MB`);
  }

  const form = new FormData();
  form.append("file", file);

  const res = await fetch("/api/images", {
    method: "POST",
    credentials: "include",
    body: form,
  });

  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to upload image"));
  }

  const body = (await res.json()) as { data?: { url?: string } };
  const url = body.data?.url;
  if (typeof url !== "string" || url.length === 0) {
    throw new Error("Server did not return image URL");
  }
  return url;
}
