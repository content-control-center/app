/**
 * Stream SSE frames from a long-lived fetch response.
 *
 * EventSource is the obvious primitive for SSE in browsers, but it doesn't
 * surface the `event:` field on the default handler — you have to register
 * a listener per event type. For a "show me everything on the all topic"
 * prototype, that's a non-starter.
 *
 * fetch + ReadableStream gives full frame visibility (id / event / data)
 * without enumerating types up front. credentials: "include" carries the
 * session cookie issued by the main app.
 *
 * Lifecycle:
 *   - onEvent({ id, event, data }) fires for each parsed frame.
 *     Heartbeat comments (": ping") are ignored.
 *   - onStatus("connecting" | "connected" | "disconnected" | "error") fires
 *     on connection state changes. Wire it to the status indicator.
 *   - The returned function aborts the fetch — call it to cancel.
 */
export function openEventStream({ url, onEvent, onStatus }) {
  const controller = new AbortController();
  const decoder = new TextDecoder();
  let buf = "";
  let cancelled = false;

  (async () => {
    onStatus?.("connecting");
    try {
      const res = await fetch(url, {
        method: "GET",
        headers: { Accept: "text/event-stream" },
        credentials: "include",
        signal: controller.signal,
      });
      if (!res.ok) {
        onStatus?.("error", `HTTP ${res.status}`);
        return;
      }
      if (!res.body) {
        onStatus?.("error", "no response body");
        return;
      }

      onStatus?.("connected");
      const reader = res.body.getReader();

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        flushFrames();
      }
      buf += decoder.decode();
      flushFrames();

      if (!cancelled) onStatus?.("disconnected");
    } catch (err) {
      if (cancelled || err.name === "AbortError") return;
      onStatus?.("error", err.message || String(err));
    }
  })();

  function flushFrames() {
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const frame = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      // Comments / heartbeats start with ":" — skip them.
      if (frame.startsWith(":")) continue;

      let id = "";
      let event = "";
      let dataStr = "";
      for (const line of frame.split("\n")) {
        if (line.startsWith("id: ")) id = line.slice(4);
        else if (line.startsWith("event: ")) event = line.slice(7);
        else if (line.startsWith("data: ")) dataStr = line.slice(6);
      }
      if (!event && !dataStr) continue;

      let data = dataStr;
      try {
        data = JSON.parse(dataStr);
      } catch {
        // not JSON — keep as raw string
      }
      onEvent?.({ id, event: event || "message", data });
    }
  }

  return () => {
    cancelled = true;
    controller.abort();
  };
}
