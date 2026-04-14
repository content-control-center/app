import { platformConfig } from "../lib/platform.jsx";

export function PostDetailModal({ card, onClose }) {
  if (!card) return null;

  const cfg = platformConfig(card.payload?.post?.platformId);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-[4px] border border-slate-200 bg-white shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between border-b border-slate-200 px-5 py-3">
          <h3 className="text-sm font-semibold text-slate-900">
            {card.payload?.post?.title || `Post #${(card.payload?.index ?? 0) + 1}`}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="text-slate-400 hover:text-slate-600 text-lg leading-none"
          >
            ×
          </button>
        </header>
        <div className="space-y-3 px-5 py-4 text-xs text-slate-700">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Platform</p>
              <div className="mt-1 flex items-center gap-1.5">
                <span className={`inline-flex items-center gap-1.5 rounded-[3px] px-2 py-0.5 text-xs font-medium ${cfg.chip}`}>
                  {cfg.icon}
                  {cfg.label}
                </span>
              </div>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Content Type</p>
              <p className="mt-0.5">{card.payload?.post?.contentType || "-"}</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Publish Date</p>
              <p className="mt-0.5">{card.payload?.post?.publishDate || "-"}</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Index</p>
              <p className="mt-0.5">{card.payload?.index ?? "-"}</p>
            </div>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Body</p>
            <p className="mt-1 max-h-64 overflow-y-auto whitespace-pre-wrap rounded-[3px] bg-slate-50 p-3 leading-relaxed">
              {card.payload?.post?.body || "-"}
            </p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Tone Notes</p>
            <p className="mt-1 whitespace-pre-wrap rounded-[3px] bg-slate-50 p-3 leading-relaxed">
              {card.payload?.post?.toneNotes || "-"}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
