import { useEffect, useMemo, useState } from "react";
import { buildPostsEndpoint } from "../lib/api.js";

// Ascending by scheduled_at; posts without a scheduled time sink to the
// bottom. Ties break on created_at so order stays stable across reloads.
function compareByScheduled(a, b) {
  const ta = a.scheduled_at ? Date.parse(a.scheduled_at) : NaN;
  const tb = b.scheduled_at ? Date.parse(b.scheduled_at) : NaN;
  const aMissing = Number.isNaN(ta);
  const bMissing = Number.isNaN(tb);
  if (aMissing && bMissing) {
    return Date.parse(a.created_at || 0) - Date.parse(b.created_at || 0);
  }
  if (aMissing) return 1;
  if (bMissing) return -1;
  if (ta !== tb) return ta - tb;
  return Date.parse(a.created_at || 0) - Date.parse(b.created_at || 0);
}

export function PostPicker({ baseUrl, campaignId, selectedId, onSelect }) {
  const [posts, setPosts] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const sortedPosts = useMemo(() => posts.slice().sort(compareByScheduled), [posts]);

  useEffect(() => {
    if (!campaignId) {
      setPosts([]);
      return;
    }

    let cancelled = false;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const url = buildPostsEndpoint(baseUrl, campaignId);
        const res = await fetch(url, { credentials: "include" });
        if (!res.ok) throw new Error(`Failed to load posts (${res.status})`);
        const data = await res.json();
        if (!cancelled) setPosts(Array.isArray(data) ? data : []);
      } catch (e) {
        if (!cancelled) setError(e.message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => { cancelled = true; };
  }, [baseUrl, campaignId]);

  if (!campaignId) {
    return (
      <div className="px-3 py-4 text-[10px] text-slate-400">
        Select a campaign to see its posts
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto">
      {loading && <p className="px-3 py-4 text-[10px] text-slate-400">Loading posts…</p>}
      {error && <p className="px-3 py-4 text-[10px] text-red-500">Error: {error}</p>}

      {!loading && !error && sortedPosts.length === 0 && (
        <p className="px-3 py-4 text-[10px] text-slate-400">No posts in this campaign</p>
      )}

      {sortedPosts.map((p) => {
        const scheduled = p.scheduled_at ? new Date(p.scheduled_at) : null;
        const scheduledLabel = scheduled && !isNaN(scheduled.getTime())
          ? scheduled.toLocaleString()
          : "Unscheduled";
        return (
          <button
            key={p.id}
            type="button"
            onClick={() => onSelect(p)}
            className={`w-full border-b border-slate-100 px-3 py-2.5 text-left transition ${
              selectedId === p.id
                ? "bg-blue-200 border-l-2 border-l-blue-700 text-blue-950"
                : "hover:bg-slate-50"
            }`}
          >
            <span className="block text-xs font-medium text-slate-700">
              {p.title || "(untitled)"}
            </span>
            <span className="block text-[10px] text-slate-400 mt-0.5">{scheduledLabel}</span>
          </button>
        );
      })}
    </div>
  );
}
