export function CampaignSidebar({ campaigns, loading, selectedId, onSelect }) {
  return (
    <div className="flex-1 overflow-y-auto">
      {loading && <p className="px-3 py-4 text-[10px] text-slate-400">Loading…</p>}

      {!loading && campaigns.length === 0 && (
        <p className="px-3 py-4 text-[10px] text-slate-400">No campaigns found</p>
      )}

      {campaigns.map((c) => (
        <button
          key={c.id}
          type="button"
          onClick={() => onSelect(c)}
          className={`w-full border-b border-slate-100 px-3 py-2.5 text-left transition ${
            selectedId === c.id
              ? "bg-blue-200 border-l-2 border-l-blue-700 text-blue-950"
              : "hover:bg-slate-50"
          }`}
        >
          <span className="block text-xs font-medium text-slate-700">{c.name}</span>
          <span className="block text-[10px] text-slate-400 truncate mt-0.5">
            {c.status || "—"}
          </span>
        </button>
      ))}
    </div>
  );
}
