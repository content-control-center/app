import { AutoTextarea } from "./AutoTextarea.jsx";
import { PlatformPicker } from "./PlatformPicker.jsx";
import { inputClass, labelClass } from "../lib/styles.js";

export function CreateCampaignTab({
  createForm,
  setCreateField,
  handleCreateCampaign,
  isCreating,
  createError,
  createSuccess,
  campaignTypes,
  campaignTypesLoading,
  campaignTypesError,
  platforms,
  platformsLoading,
  platformsError,
  selectedPlatforms,
  onTogglePlatform,
  onTogglePostType,
}) {
  const selectedType = campaignTypes.find((t) => t.id === createForm.campaign_type_id);

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <p className="text-xs text-slate-600">
        POST /api/campaigns — creates a new campaign and populates the Campaign ID field.
      </p>

      <p className="mt-3 rounded-[3px] border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
        Please login in Ogen app (<a href="http://localhost:9001" target="_blank" rel="noreferrer" className="underline">http://localhost:9001</a>) before using this prototype.
      </p>

      <form className="mt-4 space-y-3" onSubmit={handleCreateCampaign}>
        <div>
          <label className={labelClass} htmlFor="create_name">name</label>
          <input id="create_name" className={inputClass} value={createForm.name} onChange={setCreateField("name")} />
        </div>

        {/* Campaign type picker */}
        <div>
          <label className={labelClass}>campaign_type_id</label>

          {campaignTypesError && (
            <p className="mb-1 rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs text-rose-700">{campaignTypesError}</p>
          )}

          {campaignTypesLoading ? (
            <select id="create_campaign_type_id" className={inputClass} disabled>
              <option>Loading…</option>
            </select>
          ) : campaignTypes.length > 0 ? (
            <select
              id="create_campaign_type_id"
              className={inputClass}
              value={createForm.campaign_type_id}
              onChange={setCreateField("campaign_type_id")}
            >
              <option value="">— select a type —</option>
              {campaignTypes.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.label} ({t.name}){t.is_system ? " ★" : ""}
                </option>
              ))}
            </select>
          ) : (
            <input
              id="create_campaign_type_id"
              className={inputClass}
              value={createForm.campaign_type_id}
              onChange={setCreateField("campaign_type_id")}
              placeholder="paste campaign_type_id manually"
            />
          )}

          {selectedType && (
            <div className="mt-1.5 rounded-[3px] border border-slate-200 bg-slate-50 px-3 py-2 space-y-2">
              {selectedType.description && (
                <p className="text-xs text-slate-600">{selectedType.description}</p>
              )}
              {selectedType.phases && selectedType.phases.length > 0 && (
                <>
                  <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">Phases</p>
                  <ol className="space-y-0.5">
                    {selectedType.phases
                      .slice()
                      .sort((a, b) => a.sequence - b.sequence)
                      .map((p) => (
                        <li key={p.id} className="text-xs text-slate-600">
                          <span className="font-medium text-slate-700">{p.sequence}. {p.name}</span>
                          {p.purpose && <span className="text-slate-400"> — {p.purpose}</span>}
                        </li>
                      ))}
                  </ol>
                </>
              )}
            </div>
          )}
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label className={labelClass} htmlFor="create_status">status</label>
            <select id="create_status" className={inputClass} value={createForm.status} onChange={setCreateField("status")}>
              <option value="draft">draft</option>
              <option value="scheduled">scheduled</option>
              <option value="active">active</option>
              <option value="paused">paused</option>
              <option value="completed">completed</option>
              <option value="archived">archived</option>
            </select>
          </div>
        </div>

        <div>
          <label className={labelClass} htmlFor="create_description">description</label>
          <AutoTextarea id="create_description" className={inputClass} minRows={3} value={createForm.description} onChange={setCreateField("description")} />
        </div>

        <div>
          <label className={labelClass} htmlFor="create_target_persona">target_persona</label>
          <AutoTextarea id="create_target_persona" className={inputClass} value={createForm.target_persona} onChange={setCreateField("target_persona")} />
        </div>

        <div>
          <label className={labelClass} htmlFor="create_key_messages">key_messages</label>
          <AutoTextarea id="create_key_messages" className={inputClass} value={createForm.key_messages} onChange={setCreateField("key_messages")} />
        </div>

        <div>
          <label className={labelClass} htmlFor="create_tone_guidelines">tone_guidelines</label>
          <AutoTextarea id="create_tone_guidelines" className={inputClass} value={createForm.tone_guidelines} onChange={setCreateField("tone_guidelines")} />
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label className={labelClass} htmlFor="create_start_date">start_date</label>
            <input id="create_start_date" type="date" className={inputClass} value={createForm.start_date} onChange={setCreateField("start_date")} />
          </div>
          <div>
            <label className={labelClass} htmlFor="create_end_date">end_date</label>
            <input id="create_end_date" type="date" className={inputClass} value={createForm.end_date} onChange={setCreateField("end_date")} />
          </div>
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label className={labelClass} htmlFor="create_estimated_post_count">estimated_post_count</label>
            <input id="create_estimated_post_count" className={inputClass} value={createForm.estimated_post_count} onChange={setCreateField("estimated_post_count")} />
          </div>
          <div>
            <label className={labelClass} htmlFor="create_budget">budget</label>
            <input id="create_budget" className={inputClass} value={createForm.budget} onChange={setCreateField("budget")} />
          </div>
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label className={labelClass} htmlFor="create_currency">currency</label>
            <input id="create_currency" className={inputClass} value={createForm.currency} onChange={setCreateField("currency")} />
          </div>
          <div>
            <label className={labelClass} htmlFor="create_language">language</label>
            <input id="create_language" className={inputClass} value={createForm.language} onChange={setCreateField("language")} />
          </div>
        </div>

        <div className="flex items-center">
          <label className="flex cursor-pointer items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.08em] text-slate-600">
            <input type="checkbox" checked={createForm.use_pieces} onChange={setCreateField("use_pieces")} />
            use_pieces
          </label>
        </div>

        <div>
          <label className={labelClass} htmlFor="create_pieces_ids">pieces_ids (comma separated)</label>
          <input id="create_pieces_ids" className={inputClass} value={createForm.pieces_ids} onChange={setCreateField("pieces_ids")} placeholder="piece_a,piece_b" />
        </div>

        <div>
          <label className={labelClass} htmlFor="create_tag_ids">tag_ids (comma separated)</label>
          <input id="create_tag_ids" className={inputClass} value={createForm.tag_ids} onChange={setCreateField("tag_ids")} placeholder="tag_a,tag_b" />
        </div>

        <PlatformPicker
          platforms={platforms}
          platformsLoading={platformsLoading}
          platformsError={platformsError}
          selectedPlatforms={selectedPlatforms}
          onTogglePlatform={onTogglePlatform}
          onTogglePostType={onTogglePostType}
        />

        {createError && (
          <p className="rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">{createError}</p>
        )}
        {createSuccess && (
          <p className="rounded-[3px] border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-700">{createSuccess}</p>
        )}

        <div>
          <button
            type="submit"
            className="rounded-[3px] border border-slate-700 bg-slate-700 px-4 py-1.5 text-xs font-semibold text-white transition hover:bg-slate-600 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={isCreating}
          >
            {isCreating ? "Creating..." : "Create Campaign"}
          </button>
        </div>
      </form>
    </div>
  );
}
