import { useEffect, useRef, useState } from "react";

export function ChatPanel({ messages, onSend, isLoading }) {
  const [input, setInput] = useState("");
  const bottomRef = useRef(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const handleSubmit = (e) => {
    e.preventDefault();
    const text = input.trim();
    if (!text || isLoading) return;
    onSend(text);
    setInput("");
  };

  const handleKeyDown = (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Messages area */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
        {messages.length === 0 && (
          <p className="text-center text-xs text-slate-400 py-8">
            Select a post and send an instruction to the assistant.
          </p>
        )}
        {messages.map((msg, i) => (
          <div key={i} className={`fade-in-card flex items-center gap-2 ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
            <div
              className={`max-w-[85%] rounded-lg px-3 py-2 text-xs leading-relaxed ${
                msg.role === "user"
                  ? "bg-slate-700 text-white"
                  : msg.role === "error"
                    ? "bg-red-50 border border-red-200 text-red-700"
                    : "bg-white border border-slate-200 text-slate-700"
              }`}
            >
              {msg.role === "assistant" && msg.action && (
                <div className="mb-1.5 flex items-center gap-2">
                  <span
                    className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase ${
                      msg.action === "edited"
                        ? "bg-emerald-100 text-emerald-700"
                        : "bg-amber-100 text-amber-700"
                    }`}
                  >
                    {msg.action}
                  </span>
                  {msg.saveVersion && (
                    <span className="inline-block rounded bg-blue-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-blue-700">
                      version saved
                    </span>
                  )}
                </div>
              )}
              {Array.isArray(msg.toolCalls) && msg.toolCalls.length > 0 && (
                <div className="mb-2 space-y-0.5 font-mono text-[11px]">
                  {msg.toolCalls.map((tc, j) => (
                    <ToolActionLine key={`${tc.ref || tc.name}-${j}`} call={tc} />
                  ))}
                </div>
              )}
              {msg.text
                ? <p className="whitespace-pre-wrap">{msg.text}</p>
                : msg.streaming && <p className="text-slate-400 italic">thinking…</p>}
              {msg.versionNote && (
                <p className="mt-1 text-[10px] text-slate-400 italic">Version note: {msg.versionNote}</p>
              )}
            </div>
            {msg.streaming && <span className="spinner" aria-label="Assistant is responding" title="Assistant is responding" />}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Input area */}
      <form onSubmit={handleSubmit} className="border-t border-slate-200 bg-white px-4 py-3">
        <div className="flex gap-2">
          <textarea
            rows={2}
            placeholder="e.g. Expand the second paragraph with more details about pricing…"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            className="flex-1 resize-none rounded border border-slate-300 bg-white px-2.5 py-2 text-xs focus:border-slate-500 focus:outline-none"
          />
          <button
            type="submit"
            disabled={isLoading || !input.trim()}
            className="self-end rounded bg-slate-700 px-4 py-2 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
          >
            Send
          </button>
        </div>
      </form>
    </div>
  );
}

// ToolActionLine renders one row in the assistant bubble's "system action
// log" — Claude-Code-style: a coloured glyph (animated while running,
// static when done) followed by a human-readable description of the tool
// call. Shows the input when meaningful (search query, asset id) so the
// user can follow what the assistant is doing.
function ToolActionLine({ call }) {
  const isRunning = call.status !== "done";
  return (
    <div className="flex items-start gap-2 leading-tight">
      <span
        aria-hidden="true"
        className={isRunning ? "tool-bullet tool-bullet-running" : "tool-bullet tool-bullet-done"}
      >
        {isRunning ? "✱" : "●"}
      </span>
      <span className={isRunning ? "text-amber-700" : "text-slate-400"}>
        {describeToolCall(call)}
      </span>
    </div>
  );
}

const TOOL_DESCRIPTIONS = {
  listAssets: {
    running: () => "Listing attached assets…",
    done:    () => "Listed attached assets",
  },
  getAssetChunks: {
    running: (i) => `Reading ${formatChunkTarget(i)}…`,
    done:    (i) => `Read ${formatChunkTarget(i)}`,
  },
  searchAssetChunks: {
    running: (i) => `Searching ${formatAsset(i?.assetId)} for ${formatQuery(i?.query)}…`,
    done:    (i) => `Searched ${formatAsset(i?.assetId)} for ${formatQuery(i?.query)}`,
  },
  getCurrentContent: {
    running: () => "Re-reading current post…",
    done:    () => "Re-read current post",
  },
};

function describeToolCall(call) {
  const entry = TOOL_DESCRIPTIONS[call.name];
  if (!entry) return call.name; // unknown tool — show its raw name
  return (call.status === "done" ? entry.done : entry.running)(call.input);
}

function formatChunkTarget(input) {
  if (!input) return "asset";
  const asset = formatAsset(input.assetId);
  const ids = Array.isArray(input.chunkIds) ? input.chunkIds : [];
  if (ids.length === 0) return `all chunks of ${asset}`;
  if (ids.length === 1) return `1 chunk of ${asset}`;
  return `${ids.length} chunks of ${asset}`;
}

function formatAsset(id) {
  if (!id) return "an asset";
  return `asset ${id}`;
}

function formatQuery(q) {
  if (!q) return "<empty>";
  const trimmed = q.length > 60 ? q.slice(0, 60) + "…" : q;
  return `"${trimmed}"`;
}
