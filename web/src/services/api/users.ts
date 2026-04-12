/**
 * Users API — registration / user creation.
 *
 * `POST /api/users` is open during first-run setup (`setup_complete=false`)
 * and protected afterwards. The backend schema is `{name, email, password}`
 * — the frontend register form collects first/last name separately, so we
 * join them into a single `name` field here.
 */

import type { User, RegisterPayload } from "@/types/user";

/**
 * TODO: replace with real `GET /api/me` once the backend supports it.
 */
export async function getMe(): Promise<User> {
  const now = new Date().toISOString();
  return {
    id: "stub",
    firstName: "John",
    lastName: "Doe",
    email: "email@example.com",
    created_at: now,
    updated_at: now,
  };
}

export async function register(payload: RegisterPayload): Promise<User> {
  const res = await fetch("/api/users", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: `${payload.firstName} ${payload.lastName}`.trim(),
      email: payload.email,
      password: payload.password,
    }),
  });
  if (!res.ok) {
    throw new Error(await errorMessage(res, "Unable to create account"));
  }
  const body = (await res.json()) as { id: string; name: string; email: string; created_at: string; updated_at: string };
  const [firstName = "", ...rest] = body.name.split(" ");
  return { ...body, firstName, lastName: rest.join(" ") };
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
