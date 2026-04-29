export function SystemMessages({
  systemCards,
  receivedPostCount = 0,
  expectedPostCount = 0,
  isStreaming = false,
}) {
  // Posts arrive interleaved (CON-67 parallel batching), so a streaming
  // run shows the count climbing in jumps as each batch finishes rather
  // than smoothly. The progress label is the easiest place to surface
  // that — preferable to relying on the calendar filling in.
  const progressLabel = expectedPostCount > 0
    ? `${receivedPostCount} / ${expectedPostCount} posts`
    : receivedPostCount > 0
      ? `${receivedPostCount} posts`
      : null;

  const progressPct = expectedPostCount > 0
    ? Math.min(100, Math.round((receivedPostCount / expectedPostCount) * 100))
    : 0;

  return (
    <div className="border-t border-slate-200">
      <div className="flex items-center gap-2 bg-slate-50 px-4 py-2">
        <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-slate-400">
          <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
        </svg>
        <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">System Messages</span>
        {progressLabel && (
          <span className="ml-2 flex items-center gap-2">
            <span className="text-[10px] text-slate-500">{progressLabel}</span>
            {expectedPostCount > 0 && (
              <span className="h-1.5 w-24 overflow-hidden rounded-full bg-slate-200">
                <span
                  className={`block h-full transition-all duration-200 ${isStreaming ? "bg-sky-500" : "bg-emerald-500"}`}
                  style={{ width: `${progressPct}%` }}
                />
              </span>
            )}
          </span>
        )}
        <span className="ml-auto text-[10px] text-slate-400">{systemCards.length} events</span>
      </div>
      <div className="flex h-56 flex-col-reverse gap-1 overflow-y-auto bg-slate-900 px-3 py-2">
        {systemCards.length === 0 ? (
          <p className="text-[11px] text-slate-500 italic">No system events yet.</p>
        ) : (
          [...systemCards].reverse().map((card) => {
            const accent =
              card.kind === "error" ? "text-rose-400" :
              card.kind === "warning" ? "text-amber-300" :
              card.kind === "complete" ? "text-emerald-400" :
              card.kind === "request" ? "text-sky-400" :
              card.kind === "step" ? "text-amber-400" :
              "text-slate-400";
            return (
              <div key={card.id} className="flex items-start gap-2 font-mono text-[11px]">
                <span className="shrink-0 text-slate-500">{new Date(card.receivedAt).toLocaleTimeString()}</span>
                <span className={`shrink-0 font-semibold ${accent}`}>[{card.kind}]</span>
                <span className="text-slate-300 truncate">
                  {card.title}
                  {card.payload?.message ? ` — ${card.payload.message}` : ""}
                  {card.payload?.step ? ` — ${card.payload.step}` : ""}
                </span>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
