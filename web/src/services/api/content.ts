import type { Piece, CreatePiecePayload, UpdatePiecePayload } from "@/types/content";

const BASE = "/api/content-bank/pieces";

export async function listPieces(): Promise<Piece[]> {
  const res = await fetch(BASE, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch content pieces"));
  }
  return (await res.json()) as Piece[];
}

export async function getPiece(id: string): Promise<Piece> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to fetch content piece"));
  }
  return (await res.json()) as Piece;
}

export async function createPiece(payload: CreatePiecePayload): Promise<Piece> {
  const res = await fetch(BASE, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to create content piece"));
  }
  return (await res.json()) as Piece;
}

export async function updatePiece(id: string, payload: UpdatePiecePayload): Promise<Piece> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to update content piece"));
  }
  return (await res.json()) as Piece;
}

export async function deletePiece(id: string): Promise<void> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to delete content piece"));
  }
}

async function errorMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (typeof body.error === "string" && body.error.length > 0) return body.error;
  } catch {
    // fall through
  }
  return fallback;
}
