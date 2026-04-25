export function CampaignSidebar({ campaigns, loading, selectedId, onSelect, onDelete }) {
  return (
    <div className="flex-1 overflow-y-auto">
      {loading && <p className="px-3 py-4 text-[10px] text-slate-400">Loading…</p>}

      {!loading && campaigns.length === 0 && (
        <p className="px-3 py-4 text-[10px] text-slate-400">No campaigns found</p>
      )}

      {campaigns.map((c) => (
        <div
          key={c.id}
          className={`flex items-center border-b border-slate-100 transition ${
            selectedId === c.id
              ? "bg-blue-200 border-l-2 border-l-blue-700 text-blue-950"
              : "hover:bg-slate-50"
          }`}
        >
          <button
            type="button"
            onClick={() => onSelect(c)}
            className="flex-1 min-w-0 px-3 py-2.5 text-left"
          >
            <span className="block text-xs font-medium text-slate-700">{c.name}</span>
            <span className="block text-[10px] text-slate-400 truncate mt-0.5">
              {c.status || "—"}
            </span>
          </button>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onDelete(c.id);
            }}
            className="shrink-0 p-2 text-slate-300 hover:text-red-500 transition"
            title="Delete campaign"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M3 6h18" />
              <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6" />
              <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" />
            </svg>
          </button>
        </div>
      ))}
    </div>
  );
}
