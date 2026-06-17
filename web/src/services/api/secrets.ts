import type { SecretMetadata, SecretName } from "@/types/integrations";
import { errorMessage } from "./errors";

const BASE = "/api/secrets";

export async function listSecrets(): Promise<SecretMetadata[]> {
  const res = await fetch(BASE, { method: "GET", credentials: "include" });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch secrets"));
  }
  return (await res.json()) as SecretMetadata[];
}

export async function getSecret(name: SecretName): Promise<SecretMetadata | null> {
  const res = await fetch(`${BASE}/${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
  });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(await errorMessage(res, `Unable to fetch ${name}`));
  }
  return (await res.json()) as SecretMetadata;
}

export async function putSecret(name: SecretName, value: string): Promise<SecretMetadata> {
  const res = await fetch(`${BASE}/${encodeURIComponent(name)}`, {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value }),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to save secret"));
  }
  return (await res.json()) as SecretMetadata;
}

export async function deleteSecret(name: SecretName): Promise<void> {
  const res = await fetch(`${BASE}/${encodeURIComponent(name)}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok && res.status !== 404) {
    throw new Error(await errorMessage(res, "Unable to delete secret"));
  }
}
