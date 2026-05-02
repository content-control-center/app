import { useCallback, useEffect, useRef, useState } from "react";
import { EventLog } from "./components/EventLog.jsx";
import { openEventStream } from "./lib/sseReader.js";

const MAX_ENTRIES = 500;

function App() {
  const [baseUrl, setBaseUrl] = useState("");
  const [status, setStatus] = useState("idle");
  const [statusDetail, setStatusDetail] = useState("");
  const [entries, setEntries] = useState([]);
  const [paused, setPaused] = useState(false);
  const closeRef = useRef(null);
  const localIdRef = useRef(0);

  const connect = useCallback(() => {
    if (closeRef.current) {
      closeRef.current();
      closeRef.current = null;
    }
    const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
    const url = `${base}/api/events?topics=all`;
    closeRef.current = openEventStream({
      url,
      onEvent: ({ id, event, data }) => {
        const entry = {
          localId: ++localIdRef.current,
          id,
          topic: typeof data === "object" && data ? data.topic || "" : "",
          type: event,
          payload: typeof data === "object" && data ? data.payload : data,
          receivedAt: new Date(),
        };
        setEntries((prev) => {
          const next = [...prev, entry];
          if (next.length > MAX_ENTRIES) next.splice(0, next.length - MAX_ENTRIES);
          return next;
        });
      },
      onStatus: (s, detail) => {
        setStatus(s);
        setStatusDetail(detail || "");
      },
    });
  }, [baseUrl]);

  // Auto-connect on mount and whenever baseUrl changes.
  useEffect(() => {
    connect();
    return () => {
      if (closeRef.current) {
        closeRef.current();
        closeRef.current = null;
      }
    };
  }, [connect]);

  const clearEntries = () => setEntries([]);

  return (
    <div className="h-screen flex flex-col bg-[#0b0d10] text-slate-200">
      <header className="flex items-center gap-3 px-4 py-3 border-b border-slate-800 bg-[#0e1116]">
        <h1 className="text-sm font-semibold text-slate-200">
          CON-55 Event Stream prototype
          <span className="text-slate-500 font-normal ml-2">
            /api/events?topics=all
          </span>
        </h1>

        <div className="flex-1" />

        <input
          type="text"
          placeholder="Base URL (blank = same origin)"
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          className="w-56 rounded border border-slate-700 bg-[#0b0d10] text-slate-200 px-2.5 py-1 text-xs focus:border-slate-500 focus:outline-none placeholder:text-slate-600"
        />

        <button
          type="button"
          onClick={() => setPaused((p) => !p)}
          className="rounded border border-slate-700 bg-[#0b0d10] px-2.5 py-1 text-xs text-slate-300 hover:bg-slate-800"
          title="Pause auto-scroll (events still arrive)"
        >
          {paused ? "Resume scroll" : "Pause scroll"}
        </button>

        <button
          type="button"
          onClick={clearEntries}
          className="rounded border border-slate-700 bg-[#0b0d10] px-2.5 py-1 text-xs text-slate-300 hover:bg-slate-800"
        >
          Clear
        </button>

        <button
          type="button"
          onClick={connect}
          className="rounded border border-slate-700 bg-[#0b0d10] px-2.5 py-1 text-xs text-slate-300 hover:bg-slate-800"
          title="Drop the current stream and reconnect"
        >
          Reconnect
        </button>

        <StatusIndicator status={status} detail={statusDetail} />
      </header>

      <main className="flex-1 flex flex-col min-h-0">
        <EventLog entries={entries} paused={paused} />
        <footer className="border-t border-slate-800 bg-[#0e1116] px-4 py-1.5 text-[11px] text-slate-600 flex items-center gap-3">
          <span>{entries.length} event{entries.length === 1 ? "" : "s"}</span>
          {paused && <span className="text-amber-500/80">scroll paused</span>}
          <span className="flex-1" />
          <span className="text-slate-700">
            {status === "connected" ? "live" : status}
          </span>
        </footer>
      </main>
    </div>
  );
}

function StatusIndicator({ status, detail }) {
  let color = "bg-slate-500";
  let label = status;
  let pulse = false;
  switch (status) {
    case "connecting":
      color = "bg-amber-400";
      pulse = true;
      label = "connecting";
      break;
    case "connected":
      color = "bg-emerald-400";
      label = "connected";
      break;
    case "disconnected":
      color = "bg-slate-500";
      label = "disconnected";
      break;
    case "error":
      color = "bg-rose-500";
      label = detail ? `error: ${detail}` : "error";
      break;
    default:
      label = status;
  }
  return (
    <span
      className="flex items-center gap-1.5 text-[11px] text-slate-400"
      title={detail || label}
    >
      <span
        className={`inline-block w-2 h-2 rounded-full ${color} ${pulse ? "status-dot-pulse" : ""}`}
      />
      {label}
    </span>
  );
}

export default App;
