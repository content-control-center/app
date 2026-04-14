import { inputClass, labelClass } from "../lib/styles.js";

export function GenerateDraftTab({
  draftForm,
  setDraftField,
  handleGenerateDraft,
  isStreaming,
  handleStop,
  draftError,
  draftEndpointPreview,
}) {
  return (
    <div className="flex-1 overflow-y-auto p-6">
      <p className="text-xs text-slate-600">
        Stream content plan generation events for an existing campaign.
      </p>
      <p className="mt-4 break-all rounded-[3px] border border-slate-200 bg-slate-100 px-3 py-2 text-xs text-slate-700">
        POST {draftEndpointPreview}
      </p>

      <form className="mt-4 space-y-3" onSubmit={handleGenerateDraft}>
        <div>
          <label className={labelClass} htmlFor="baseUrl">baseUrl (optional)</label>
          <input
            id="baseUrl"
            className={inputClass}
            value={draftForm.baseUrl}
            onChange={setDraftField("baseUrl")}
            placeholder="leave empty to use /api dev proxy -> localhost:9002"
          />
        </div>

        <div>
          <label className={labelClass} htmlFor="campaignId">campaignId</label>
          <input
            id="campaignId"
            className={inputClass}
            value={draftForm.campaignId}
            onChange={setDraftField("campaignId")}
            placeholder="sqid campaign id (auto-filled on create)"
          />
        </div>

        {draftError && (
          <p className="rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">{draftError}</p>
        )}

        <div className="flex gap-2">
          <button
            type="submit"
            className="rounded-[3px] border border-slate-700 bg-slate-700 px-4 py-1.5 text-xs font-semibold text-white transition hover:bg-slate-600 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={isStreaming}
          >
            {isStreaming ? "Streaming..." : "Generate Content Plan"}
          </button>
          <button
            type="button"
            className="rounded-[3px] border border-slate-300 bg-white px-4 py-1.5 text-xs font-semibold text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={handleStop}
            disabled={!isStreaming}
          >
            Stop
          </button>
        </div>
      </form>
    </div>
  );
}
