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
    <div className="flex h-full flex-col">
      {/* Messages area */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
        {messages.length === 0 && (
          <p className="text-center text-xs text-slate-400 py-8">
            Select a post and send an instruction to the assistant.
          </p>
        )}
        {messages.map((msg, i) => (
          <div key={i} className={`fade-in-card flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
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
              <p className="whitespace-pre-wrap">{msg.text}</p>
              {msg.versionNote && (
                <p className="mt-1 text-[10px] text-slate-400 italic">Version note: {msg.versionNote}</p>
              )}
            </div>
          </div>
        ))}
        {isLoading && (
          <div className="flex justify-start fade-in-card">
            <div className="rounded-lg border border-slate-200 bg-white px-3 py-2.5 text-xs text-slate-400 flex items-center gap-1.5">
              <span className="thinking-dots flex gap-0.5">
                <span className="dot" />
                <span className="dot" />
                <span className="dot" />
              </span>
              Thinking…
            </div>
          </div>
        )}
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
