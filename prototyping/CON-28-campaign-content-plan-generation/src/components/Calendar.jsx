import { MONTH_NAMES, DAY_NAMES, calendarKey } from "../lib/calendar.js";
import { platformConfig } from "../lib/platform.jsx";

export function Calendar({
  calendarDate,
  setCalendarDate,
  postsByDay,
  dragOverKey,
  setDragOverKey,
  onDrop,
  onCardClick,
}) {
  const year = calendarDate.getFullYear();
  const month = calendarDate.getMonth();
  const firstDow = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const today = new Date();

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-6">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900">
          {MONTH_NAMES[month]} {year}
        </h2>
        <div className="flex gap-1">
          <button
            type="button"
            onClick={() => setCalendarDate((d) => new Date(d.getFullYear(), d.getMonth() - 1, 1))}
            className="rounded-[3px] border border-slate-200 bg-white px-2.5 py-1 text-xs text-slate-600 hover:bg-slate-50"
          >
            ‹
          </button>
          <button
            type="button"
            onClick={() => setCalendarDate(new Date())}
            className="rounded-[3px] border border-slate-200 bg-white px-2.5 py-1 text-xs text-slate-600 hover:bg-slate-50"
          >
            Today
          </button>
          <button
            type="button"
            onClick={() => setCalendarDate((d) => new Date(d.getFullYear(), d.getMonth() + 1, 1))}
            className="rounded-[3px] border border-slate-200 bg-white px-2.5 py-1 text-xs text-slate-600 hover:bg-slate-50"
          >
            ›
          </button>
        </div>
      </div>

      <div className="grid grid-cols-7 border-l border-t border-slate-200">
        {DAY_NAMES.map((d) => (
          <div key={d} className="border-b border-r border-slate-200 bg-slate-50 px-2 py-1.5 text-center text-[10px] font-semibold uppercase tracking-wider text-slate-400">
            {d}
          </div>
        ))}

        {Array.from({ length: firstDow }, (_, i) => (
          <div key={`empty-${i}`} className="min-h-[120px] border-b border-r border-slate-200 bg-slate-50/40" />
        ))}

        {Array.from({ length: daysInMonth }, (_, i) => {
          const day = i + 1;
          const cellDate = new Date(year, month, day);
          const key = calendarKey(cellDate);
          const posts = postsByDay[key] || [];
          const isToday =
            today.getFullYear() === year &&
            today.getMonth() === month &&
            today.getDate() === day;

          return (
            <div
              key={day}
              className={`min-h-[120px] border-b border-r border-slate-200 p-1.5 transition-colors ${
                dragOverKey === key ? "bg-blue-50 ring-1 ring-inset ring-blue-300" : "bg-white"
              }`}
              onDragOver={(e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
                setDragOverKey(key);
              }}
              onDragLeave={(e) => {
                if (!e.currentTarget.contains(e.relatedTarget)) setDragOverKey(null);
              }}
              onDrop={(e) => {
                e.preventDefault();
                const cardId = Number(e.dataTransfer.getData("text/plain"));
                if (cardId) onDrop(cardId, new Date(year, month, day, 12, 0, 0));
                setDragOverKey(null);
              }}
            >
              <span className={`mb-1 flex h-5 w-5 items-center justify-center rounded-full text-[11px] font-semibold ${
                isToday ? "bg-slate-800 text-white" : "text-slate-500"
              }`}>
                {day}
              </span>
              <div className="space-y-0.5">
                {posts.slice(0, 4).map((card) => {
                  const cfg = platformConfig(card.payload?.post?.platformId);
                  return (
                    <button
                      key={card.id}
                      type="button"
                      draggable
                      onDragStart={(e) => {
                        e.dataTransfer.setData("text/plain", String(card.id));
                        e.dataTransfer.effectAllowed = "move";
                      }}
                      onClick={() => onCardClick(card)}
                      className={`fade-in-card flex w-full cursor-grab items-start gap-1.5 rounded-[2px] px-2 py-1 text-left text-[11px] font-medium leading-snug transition active:cursor-grabbing ${cfg.chip}`}
                    >
                      <span className="mt-0.5 shrink-0">{cfg.icon}</span>
                      <span>{card.payload?.post?.title || `Post #${(card.payload?.index ?? 0) + 1}`}</span>
                    </button>
                  );
                })}
                {posts.length > 4 && (
                  <button
                    type="button"
                    onClick={() => onCardClick(posts[4])}
                    className="w-full text-left text-[10px] text-slate-400 hover:text-slate-600"
                  >
                    +{posts.length - 4} more
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
