/**
 * Thrown when a request never reached the backend — the `fetch` promise itself
 * rejected (server unreachable, connection refused, DNS failure). This is
 * categorically different from an HTTP error response: an HTTP response, even a
 * 4xx/5xx, means the server is up and answering. The root route guard relies on
 * this distinction to tell a "server is down" boot apart from a genuine fresh
 * install (which returns a real response saying setup isn't complete).
 */
export class ServerUnavailableError extends Error {
  constructor() {
    super("The server is unreachable");
    this.name = "ServerUnavailableError";
  }
}

/**
 * Like `fetch`, but converts a network-level rejection into a typed
 * `ServerUnavailableError`. A returned `Response` (any status) passes through
 * untouched, so callers keep handling HTTP errors as they normally would.
 */
export async function fetchOrThrowUnavailable(
  input: RequestInfo,
  init?: RequestInit
): Promise<Response> {
  try {
    return await fetch(input, init);
  } catch {
    throw new ServerUnavailableError();
  }
}

/**
 * Extracts a human-readable error message from a non-OK API response. Prefers
 * the backend's `{ error: string }` JSON body; falls back to `fallback` when the
 * body is absent, malformed, or has no error string.
 */
export async function errorMessage(
  res: Response,
  fallback: string
): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (typeof body.error === "string" && body.error.length > 0) return body.error;
  } catch {
    // fall through
  }
  return fallback;
}
