import { platformConfig } from "../lib/platform.jsx";

export function PlatformPicker({
  platforms,
  platformsLoading,
  platformsError,
  selectedPlatforms,
  onTogglePlatform,
  onTogglePostType,
}) {
  return (
    <div>
      <span className="mb-2 block text-[10px] font-semibold uppercase tracking-[0.08em] text-slate-600">
        target platforms
      </span>

      {platformsError && (
        <p className="mb-2 rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {platformsError}
        </p>
      )}

      {platformsLoading ? (
        <p className="rounded-[3px] border border-dashed border-slate-200 px-3 py-4 text-center text-[11px] text-slate-400">
          Loading…
        </p>
      ) : platforms.length === 0 ? (
        <p className="rounded-[3px] border border-dashed border-slate-200 px-3 py-4 text-center text-[11px] text-slate-400">
          No platforms available.
        </p>
      ) : (
        <div className="space-y-2">
          {platforms.map((platform) => {
            const cfg = platformConfig(platform.id);
            const isEnabled = Boolean(selectedPlatforms[platform.id]);
            const selectedTypes = selectedPlatforms[platform.id] ?? [];
            const postTypeEntries = Object.entries(platform.post_types ?? {});

            return (
              <div
                key={platform.id}
                className={`rounded-[3px] border transition ${isEnabled ? "border-slate-300 bg-white" : "border-slate-200 bg-slate-50"}`}
              >
                <button
                  type="button"
                  onClick={() => onTogglePlatform(platform.id)}
                  className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left"
                >
                  <span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-white ${isEnabled ? "bg-slate-700" : "bg-slate-300"}`}>
                    {isEnabled ? (
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" width="12" height="12">
                        <polyline points="20 6 9 17 4 12"/>
                      </svg>
                    ) : (
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" width="12" height="12">
                        <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
                      </svg>
                    )}
                  </span>
                  <span className={`flex items-center gap-1.5 text-xs font-semibold ${isEnabled ? "text-slate-800" : "text-slate-400"}`}>
                    <span className={isEnabled ? "text-current" : "text-slate-300"}>{cfg.icon}</span>
                    {platform.name}
                  </span>
                  {isEnabled && selectedTypes.length > 0 && (
                    <span className="ml-auto text-[10px] text-slate-400">
                      {selectedTypes.length} type{selectedTypes.length !== 1 ? "s" : ""}
                    </span>
                  )}
                </button>

                {isEnabled && (
                  <div className="flex flex-wrap gap-1.5 border-t border-slate-100 px-3 pb-3 pt-2">
                    {postTypeEntries.map(([key, label]) => {
                      const active = selectedTypes.includes(key);
                      return (
                        <button
                          key={key}
                          type="button"
                          onClick={() => onTogglePostType(platform.id, key)}
                          className={`rounded-[3px] border px-2 py-0.5 text-[10px] font-medium transition ${
                            active
                              ? `${cfg.chip} border-current`
                              : "border-slate-200 bg-white text-slate-500 hover:border-slate-300 hover:text-slate-700"
                          }`}
                        >
                          {label}
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
