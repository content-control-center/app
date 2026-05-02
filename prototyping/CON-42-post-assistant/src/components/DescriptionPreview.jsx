export function DescriptionPreview({ post, description, versions, onCreateVersion }) {
  if (!post) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-slate-400">
        No post selected
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Post header */}
      <div className="border-b border-slate-200 bg-slate-50 px-4 py-2.5">
        <h3 className="text-xs font-semibold text-slate-700">{post.title || "(untitled)"}</h3>
        <p className="text-[10px] text-slate-400 mt-0.5">ID: {post.id}</p>
      </div>

      {/* Description */}
      <div className="flex-1 overflow-y-auto px-4 py-3">
        <div className="flex items-center justify-between mb-2">
          <h4 className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Current Description</h4>
          <button
            type="button"
            onClick={onCreateVersion}
            className="rounded border border-slate-300 bg-white px-2 py-1 text-[10px] font-medium text-slate-600 hover:bg-slate-50"
          >
            Save Version
          </button>
        </div>
        <div className="rounded border border-slate-200 bg-white p-3 text-xs leading-relaxed text-slate-700 whitespace-pre-wrap">
          {description || <span className="italic text-slate-400">Empty description</span>}
        </div>
      </div>

      {/* Versions */}
      {versions.length > 0 && (
        <div className="border-t border-slate-200 px-4 py-2.5 max-h-36 overflow-y-auto">
          <h4 className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
            Versions ({versions.length})
          </h4>
          <ul className="space-y-1">
            {versions.map((v) => (
              <li
                key={v.id}
                className="flex items-center gap-2 rounded bg-slate-50 px-2 py-1 text-[10px] text-slate-600"
              >
                <span className="font-mono font-semibold">v{v.version_number}</span>
                <span className="flex-1 truncate">{v.note || "—"}</span>
                <span className="shrink-0 text-slate-400">{v.creator}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
