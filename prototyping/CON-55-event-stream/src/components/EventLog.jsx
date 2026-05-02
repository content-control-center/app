import { useEffect, useRef } from "react";

/**
 * Dark terminal-style log of incoming events. Per the brief:
 *   - "key" = topic name, rendered as a slightly-lighter label.
 *   - "value" = message body (payload), rendered as JSON below.
 *
 * Auto-scrolls to the bottom whenever a new entry arrives.
 */
export function EventLog({ entries, paused }) {
  const bottomRef = useRef(null);

  useEffect(() => {
    if (paused) return;
    bottomRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [entries.length, paused]);

  if (entries.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-slate-600 text-xs">
        Waiting for events on topic{" "}
        <span className="text-slate-400 ml-1">all</span>…
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto px-4 py-3 space-y-2">
      {entries.map((entry) => (
        <EventRow key={entry.localId} entry={entry} />
      ))}
      <div ref={bottomRef} />
    </div>
  );
}

function EventRow({ entry }) {
  const { topic, type, payload, receivedAt, id } = entry;

  return (
    <div className="stream-row leading-snug">
      <div className="flex items-baseline gap-3 flex-wrap">
        <span className="text-slate-600 text-[11px] tabular-nums">
          {formatTime(receivedAt)}
        </span>
        {/* Topic — the "key" label. Slightly lighter than body text. */}
        <span className="text-slate-200 font-medium">{topic || "(no topic)"}</span>
        {/* Event type — secondary tag. Distinguishable but quiet. */}
        {type && type !== "message" && (
          <span className="text-emerald-400/80 text-[11px] uppercase tracking-wider">
            {type}
          </span>
        )}
        {id && (
          <span className="text-slate-700 text-[11px]" title={id}>
            #{shortID(id)}
          </span>
        )}
      </div>
      {/* Payload — the "value". Indent under the topic line. */}
      <pre className="text-slate-400 text-[12px] whitespace-pre-wrap break-all pl-2 mt-0.5 border-l border-slate-800">
        {formatPayload(payload)}
      </pre>
    </div>
  );
}

function formatTime(d) {
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  const ss = String(d.getSeconds()).padStart(2, "0");
  const ms = String(d.getMilliseconds()).padStart(3, "0");
  return `${hh}:${mm}:${ss}.${ms}`;
}

function shortID(id) {
  if (!id) return "";
  return id.length > 8 ? id.slice(0, 8) : id;
}

function formatPayload(p) {
  if (p === null || p === undefined) return "(empty)";
  if (typeof p === "string") return p;
  try {
    return JSON.stringify(p, null, 2);
  } catch {
    return String(p);
  }
}
