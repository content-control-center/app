import { useEffect, useState } from "react";

function Field({ label, value }) {
  if (!value) return null;
  return (
    <div>
      <dt className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">{label}</dt>
      <dd className="mt-0.5 text-xs text-slate-700">{value}</dd>
    </div>
  );
}

function formatDate(raw) {
  if (!raw) return null;
  const d = new Date(raw);
  if (isNaN(d.getTime())) return raw;
  return d.toLocaleString();
}

function PostTab({ post, versions, onCreateVersion }) {
  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-semibold text-slate-800">{post.title || "(untitled)"}</h3>
        <p className="text-[10px] text-slate-400 mt-0.5 font-mono">{post.id}</p>
      </div>

      <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5">
        <Field label="Status" value={post.status} />
        <Field label="Platform" value={post.platform?.name || post.platform_id} />
        <Field label="Post Type" value={post.platform_post_type} />
        <Field label="Phase" value={post.campaign_type_phase?.name} />
        <Field label="CTA Type" value={post.cta_type} />
        <Field label="CTA URL" value={post.cta_url} />
        <Field label="Scheduled" value={formatDate(post.scheduled_at)} />
        <Field label="Published" value={formatDate(post.published_at)} />
        <Field label="Created" value={formatDate(post.created_at)} />
        <Field label="Updated" value={formatDate(post.updated_at)} />
      </dl>

      {post.target_audience_notes && (
        <div>
          <h4 className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 mb-1">Target Audience</h4>
          <p className="rounded border border-slate-200 bg-white p-2.5 text-xs leading-relaxed text-slate-700 whitespace-pre-wrap">
            {post.target_audience_notes}
          </p>
        </div>
      )}

      {versions.length > 0 && (
        <div>
          <h4 className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 mb-1">
            Versions ({versions.length})
          </h4>
          <ul className="space-y-1">
            {versions.map((v) => (
              <li
                key={v.id}
                className="flex items-center gap-2 rounded bg-slate-50 px-2.5 py-1.5 text-[10px] text-slate-600"
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

function CampaignTab({ campaign }) {
  if (!campaign) {
    return <p className="text-xs text-slate-400 py-4">No campaign data available</p>;
  }

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-semibold text-slate-800">{campaign.name}</h3>
        {campaign.description && (
          <p className="mt-1 text-xs leading-relaxed text-slate-600 whitespace-pre-wrap">{campaign.description}</p>
        )}
      </div>

      <dl className="grid grid-cols-2 gap-x-4 gap-y-2.5">
        <Field label="Status" value={campaign.status} />
        <Field label="Language" value={campaign.language} />
        <Field label="Start" value={formatDate(campaign.start_date)} />
        <Field label="End" value={formatDate(campaign.end_date)} />
      </dl>

      {campaign.target_persona && (
        <div>
          <dt className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Target Persona</dt>
          <dd className="mt-0.5 text-xs text-slate-700 whitespace-pre-wrap">{campaign.target_persona}</dd>
        </div>
      )}

      {campaign.key_messages && (
        <div>
          <dt className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Key Messages</dt>
          <dd className="mt-0.5 text-xs text-slate-700 whitespace-pre-wrap">{campaign.key_messages}</dd>
        </div>
      )}

      {campaign.tone_guidelines && (
        <div>
          <dt className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Tone Guidelines</dt>
          <dd className="mt-0.5 text-xs text-slate-700 whitespace-pre-wrap">{campaign.tone_guidelines}</dd>
        </div>
      )}
    </div>
  );
}

function PhaseTab({ phase }) {
  if (!phase) {
    return <p className="text-xs text-slate-400 py-4">No phase assigned to this post</p>;
  }

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-semibold text-slate-800">{phase.name}</h3>
        {phase.purpose && (
          <p className="mt-1 text-xs leading-relaxed text-slate-600 whitespace-pre-wrap">{phase.purpose}</p>
        )}
      </div>
      <Field label="Sequence" value={phase.sequence} />
    </div>
  );
}

const TYPE_LABELS = {
  md:  { text: "MD",  cls: "bg-violet-100 text-violet-700" },
  pdf: { text: "PDF", cls: "bg-orange-100 text-orange-700" },
};

function AssetCard({ asset, action }) {
  const typeLabel = TYPE_LABELS[asset.type?.toLowerCase()];

  return (
    <div className="rounded border border-slate-200 bg-white p-3">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex items-center gap-1.5">
          <h4 className="text-xs font-semibold text-slate-800">{asset.title || asset.name || "(untitled)"}</h4>
          {typeLabel && (
            <span className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${typeLabel.cls}`}>{typeLabel.text}</span>
          )}
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {action}
        </div>
      </div>
      <p className="text-[10px] text-slate-400 mt-0.5 font-mono">{asset.id}</p>
    </div>
  );
}

function AssetsTab({ assets, baseUrl, post, onPostUpdated }) {
  const attached = Array.isArray(assets) ? assets : [];
  const attachedIds = new Set(attached.map((a) => a.id));

  const [allAssets, setAllAssets] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [availableOpen, setAvailableOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      try {
        const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
        const res = await fetch(`${base}/api/content-bank/assets`, { credentials: "include" });
        if (!res.ok) throw new Error(`${res.status}`);
        const data = await res.json();
        if (!cancelled) setAllAssets(Array.isArray(data) ? data : []);
      } catch (e) {
        if (!cancelled) setError(e.message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => { cancelled = true; };
  }, [baseUrl]);

  const updateAssetIds = async (newIds) => {
    setSaving(true);
    setError("");
    try {
      const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
      const res = await fetch(`${base}/api/posts/${post.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          campaign_id: post.campaign_id,
          platform_id: post.platform_id,
          platform_post_type: post.platform_post_type,
          title: post.title,
          content: post.content,
          media_urls: post.media_urls || [],
          scheduled_at: post.scheduled_at,
          published_at: post.published_at,
          status: post.status,
          cta_type: post.cta_type,
          cta_url: post.cta_url,
          target_audience_notes: post.target_audience_notes,
          used_asset_ids: newIds,
          campaign_type_phase_id: post.campaign_type_phase_id,
        }),
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(`Failed (${res.status}): ${body}`);
      }
      onPostUpdated?.();
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const handleAttach = (assetId) => {
    const newIds = [...(post.used_asset_ids || []), assetId];
    updateAssetIds(newIds);
  };

  const handleDetach = (assetId) => {
    const newIds = (post.used_asset_ids || []).filter((id) => id !== assetId);
    updateAssetIds(newIds);
  };

  const available = allAssets.filter((a) => !attachedIds.has(a.id));

  return (
    <div className="space-y-4">
      {error && <p className="text-xs text-red-600">{error}</p>}

      {/* Attached assets */}
      <div>
        <h4 className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 mb-2">
          Attached ({attached.length})
        </h4>
        {attached.length === 0 ? (
          <p className="text-xs text-slate-400">No assets attached</p>
        ) : (
          <div className="space-y-2">
            {attached.map((a) => (
              <AssetCard
                key={a.id}
                asset={a}
                action={
                  <button
                    type="button"
                    onClick={() => handleDetach(a.id)}
                    disabled={saving}
                    className="rounded bg-red-50 px-1.5 py-0.5 text-[10px] font-medium text-red-600 hover:bg-red-100 disabled:opacity-50"
                  >
                    Detach
                  </button>
                }
              />
            ))}
          </div>
        )}
      </div>

      <hr className="border-slate-200" />

      {/* Available assets — collapsible */}
      <div>
        <button
          type="button"
          onClick={() => setAvailableOpen((v) => !v)}
          className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-400 hover:text-slate-600 transition"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24"
            fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
            className={`transition-transform ${availableOpen ? "rotate-90" : ""}`}
          >
            <path d="m9 18 6-6-6-6" />
          </svg>
          Available {!loading && `(${available.length})`}
        </button>
        {availableOpen && (
          <div className="mt-2">
            {loading && <p className="text-xs text-slate-400">Loading assets…</p>}
            {!loading && available.length === 0 && (
              <p className="text-xs text-slate-400">No more assets available</p>
            )}
            {!loading && available.length > 0 && (
              <div className="space-y-2">
                {available.map((a) => (
                  <AssetCard
                    key={a.id}
                    asset={a}
                    action={
                      <button
                        type="button"
                        onClick={() => handleAttach(a.id)}
                        disabled={saving}
                        className="rounded bg-blue-50 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 hover:bg-blue-100 disabled:opacity-50"
                      >
                        Attach
                      </button>
                    }
                  />
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

const TABS = [
  { key: "post", label: "Post" },
  { key: "campaign", label: "Campaign" },
  { key: "phase", label: "Phase" },
  { key: "assets", label: "Assets" },
];

export function PostDetails({ post, versions: rawVersions, onCreateVersion, baseUrl, onPostUpdated }) {
  const versions = Array.isArray(rawVersions) ? rawVersions : [];
  const [activeTab, setActiveTab] = useState("post");

  if (!post) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-slate-400">
        Select a post to view details
      </div>
    );
  }

  const tabClass = (key) =>
    `px-3 py-2 text-[11px] font-semibold uppercase tracking-wider transition border-b-2 ${
      activeTab === key
        ? "border-slate-700 text-slate-900 bg-white"
        : "border-transparent text-slate-400 hover:text-slate-600 hover:bg-slate-50"
    }`;

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Tab bar */}
      <div className="flex border-b border-slate-200 bg-slate-50">
        {TABS.map((t) => {
          const count = t.key === "assets" ? (post.used_assets?.length || 0) : 0;
          return (
            <button key={t.key} type="button" className={tabClass(t.key)} onClick={() => setActiveTab(t.key)}>
              {t.label}
              {t.key === "assets" && count > 0 && (
                <span className="ml-1 rounded-full bg-blue-100 px-1.5 py-0.5 text-[9px] font-bold text-blue-700">{count}</span>
              )}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto px-4 py-3">
        {activeTab === "post" && (
          <PostTab post={post} versions={versions} onCreateVersion={onCreateVersion} />
        )}
        {activeTab === "campaign" && <CampaignTab campaign={post.campaign} />}
        {activeTab === "phase" && <PhaseTab phase={post.campaign_type_phase} />}
        {activeTab === "assets" && <AssetsTab assets={post.used_assets} baseUrl={baseUrl} post={post} onPostUpdated={onPostUpdated} />}
      </div>
    </div>
  );
}
