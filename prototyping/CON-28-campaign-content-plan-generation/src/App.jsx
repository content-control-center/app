import { useEffect, useMemo, useRef, useState } from "react";

const initialCreateForm = {
  cs_session: "",
  name: "Importance of Modern Haskell in AI Development",
  description:
    "Show AI product teams and research engineers why modern Haskell is practical for building reliable model-serving pipelines, interpretable tooling, and formally safer data transformations.",
  target_persona:
    "Senior ML engineers, platform engineers, and technical founders evaluating strongly typed languages for production AI systems.",
  key_messages:
    "Modern Haskell enables correctness-by-construction, expressive abstractions for model workflows, and maintainable AI infrastructure with fewer runtime surprises.",
  tone_guidelines:
    "Clear, technically grounded, and pragmatic. Emphasize engineering trade-offs, concrete examples, and practical adoption paths.",
  use_pieces: false,
  pieces_ids: "",
  objective: "awareness",
  status: "draft",
  start_date: "2026-05-01",
  end_date: "2026-05-31",
  estimated_post_count: "10",
  budget: "1500",
  currency: "USD",
  language: "en",
  tag_ids: "",
};

const initialDraftForm = {
  baseUrl: "",
  campaignId: "",
};

function normalizeList(value) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseOptionalNumber(value, label) {
  if (value === "") {
    return null;
  }
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    throw new Error(`${label} must be a valid number`);
  }
  return parsed;
}

function toISOStringOrNull(dateValue) {
  if (!dateValue) {
    return null;
  }
  const parsed = new Date(dateValue);
  if (Number.isNaN(parsed.getTime())) {
    return null;
  }
  return parsed.toISOString();
}


const MONTH_NAMES = [
  "January","February","March","April","May","June",
  "July","August","September","October","November","December",
];
const DAY_NAMES = ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"];

const PLATFORM_CONFIG = {
  "81mUCmc2xsKd": {
    label: "Twitter / X",
    chip: "bg-slate-100 text-slate-800 hover:bg-slate-200",
    dot: "#0f1419",
    icon: (
      <svg viewBox="0 0 24 24" fill="currentColor" width="10" height="10" className="shrink-0">
        <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-4.714-6.231-5.401 6.231H2.747l7.73-8.835L1.254 2.25H8.08l4.253 5.622zm-1.161 17.52h1.833L7.084 4.126H5.117z"/>
      </svg>
    ),
  },
  "8S8bWQTG6qD": {
    label: "YouTube",
    chip: "bg-red-50 text-red-700 hover:bg-red-100",
    dot: "#ff0000",
    icon: (
      <svg viewBox="0 0 24 24" fill="currentColor" width="10" height="10" className="shrink-0">
        <path d="M23 7s-.3-2-1.2-2.8c-1.1-1.2-2.4-1.2-3-1.3C16.1 2.8 12 2.8 12 2.8s-4.1 0-6.8.2c-.6 0-1.9.1-3 1.3C1.3 5 1 7 1 7S.7 9.3.7 11.5v2.1c0 2.2.3 4.5.3 4.5s.3 2 1.2 2.8c1.1 1.2 2.6 1.1 3.3 1.2C7.5 22.2 12 22.2 12 22.2s4.1 0 6.8-.3c.6-.1 1.9-.1 3-1.3.9-.8 1.2-2.8 1.2-2.8s.3-2.2.3-4.5v-2C23.3 9.3 23 7 23 7zM9.7 15.5V8.4l8.1 3.6-8.1 3.5z"/>
      </svg>
    ),
  },
  "AXqWG7U2qnpt": {
    label: "LinkedIn",
    chip: "bg-blue-50 text-blue-700 hover:bg-blue-100",
    dot: "#0a66c2",
    icon: (
      <svg viewBox="0 0 24 24" fill="currentColor" width="10" height="10" className="shrink-0">
        <path d="M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433a2.062 2.062 0 0 1-2.063-2.065 2.064 2.064 0 1 1 2.063 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.451C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.222 0h.003z"/>
      </svg>
    ),
  },
  "pQ4yxT3SuE57": {
    label: "Threads",
    chip: "bg-zinc-100 text-zinc-800 hover:bg-zinc-200",
    dot: "#101010",
    icon: (
      <svg viewBox="0 0 24 24" fill="currentColor" width="10" height="10" className="shrink-0">
        <path d="M12.186 24h-.007c-3.581-.024-6.334-1.205-8.184-3.509C2.35 18.44 1.5 15.586 1.472 12.01v-.017c.03-3.579.879-6.43 2.525-8.482C5.845 1.205 8.6.024 12.18 0h.014c2.746.02 5.043.725 6.826 2.098 1.677 1.29 2.858 3.13 3.509 5.467l-2.04.569c-1.104-3.96-3.898-5.984-8.304-6.015-2.91.022-5.11.936-6.54 2.717C4.307 6.504 3.616 8.914 3.589 12c.027 3.086.718 5.496 2.057 7.164 1.43 1.783 3.631 2.698 6.54 2.717 2.623-.02 4.358-.631 5.8-2.045 1.647-1.613 1.618-3.593 1.09-4.798-.31-.71-.873-1.3-1.634-1.75-.192 1.352-.622 2.446-1.284 3.272-.886 1.102-2.14 1.704-3.73 1.79-1.202.065-2.361-.218-3.259-.801-1.063-.689-1.685-1.74-1.752-2.964-.065-1.19.408-2.285 1.33-3.082.88-.76 2.119-1.207 3.583-1.291a13.853 13.853 0 0 1 3.02.142c-.126-.742-.375-1.332-.75-1.757-.513-.583-1.318-.878-2.397-.883h-.036c-.874 0-2.022.238-2.764 1.168l-1.58-1.36C7.824 3.807 9.4 3.217 11.395 3.21h.039c1.662.007 3.015.49 3.926 1.419 1.041 1.063 1.495 2.604 1.352 4.578a6.967 6.967 0 0 1 1.606 1.222c1.04 1.046 1.619 2.384 1.633 3.768.032 3.007-1.697 5.98-5.137 7.065-.95.3-1.99.456-3.09.48l-.538.258zM12.58 10.86c-.214 0-.43.005-.648.017-1.074.062-1.921.364-2.451.873-.476.459-.702 1.073-.666 1.78.068 1.262 1.15 2.01 2.86 1.92 1.102-.06 1.937-.45 2.48-1.156.604-.784.862-1.954.762-3.477a11.687 11.687 0 0 0-2.337.043z"/>
      </svg>
    ),
  },
  "rzgpTkARLH0L": {
    label: "Instagram",
    chip: "bg-fuchsia-50 text-fuchsia-700 hover:bg-fuchsia-100",
    dot: "#e1306c",
    icon: (
      <svg viewBox="0 0 24 24" fill="currentColor" width="10" height="10" className="shrink-0">
        <path d="M12 2.163c3.204 0 3.584.012 4.85.07 3.252.148 4.771 1.691 4.919 4.919.058 1.265.069 1.645.069 4.849 0 3.205-.012 3.584-.069 4.849-.149 3.225-1.664 4.771-4.919 4.919-1.266.058-1.644.07-4.85.07-3.204 0-3.584-.012-4.849-.07-3.26-.149-4.771-1.699-4.919-4.92-.058-1.265-.07-1.644-.07-4.849 0-3.204.013-3.583.07-4.849.149-3.227 1.664-4.771 4.919-4.919 1.266-.057 1.645-.069 4.849-.069zM12 0C8.741 0 8.333.014 7.053.072 2.695.272.273 2.69.073 7.052.014 8.333 0 8.741 0 12c0 3.259.014 3.668.072 4.948.2 4.358 2.618 6.78 6.98 6.98C8.333 23.986 8.741 24 12 24c3.259 0 3.668-.014 4.948-.072 4.354-.2 6.782-2.618 6.979-6.98.059-1.28.073-1.689.073-4.948 0-3.259-.014-3.667-.072-4.947-.196-4.354-2.617-6.78-6.979-6.98C15.668.014 15.259 0 12 0zm0 5.838a6.162 6.162 0 1 0 0 12.324 6.162 6.162 0 0 0 0-12.324zM12 16a4 4 0 1 1 0-8 4 4 0 0 1 0 8zm6.406-11.845a1.44 1.44 0 1 0 0 2.881 1.44 1.44 0 0 0 0-2.881z"/>
      </svg>
    ),
  },
  "zBU1zqVICGfk": {
    label: "Facebook",
    chip: "bg-sky-50 text-sky-700 hover:bg-sky-100",
    dot: "#1877f2",
    icon: (
      <svg viewBox="0 0 24 24" fill="currentColor" width="10" height="10" className="shrink-0">
        <path d="M24 12.073c0-6.627-5.373-12-12-12s-12 5.373-12 12c0 5.99 4.388 10.954 10.125 11.854v-8.385H7.078v-3.47h3.047V9.43c0-3.007 1.792-4.669 4.533-4.669 1.312 0 2.686.235 2.686.235v2.953H15.83c-1.491 0-1.956.925-1.956 1.874v2.25h3.328l-.532 3.47h-2.796v8.385C19.612 23.027 24 18.062 24 12.073z"/>
      </svg>
    ),
  },
};

function platformConfig(platformId) {
  return PLATFORM_CONFIG[platformId] ?? {
    label: platformId || "Unknown",
    chip: "bg-indigo-50 text-indigo-700 hover:bg-indigo-100",
    dot: "#6366f1",
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" width="10" height="10" className="shrink-0">
        <circle cx="12" cy="12" r="10"/>
      </svg>
    ),
  };
}

function calendarKey(date) {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

function buildGenerateDraftEndpoint(baseUrl, campaignId, encodeCampaignId = true) {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
  const normalizedCampaignId = campaignId.trim();
  const campaignIdSegment = encodeCampaignId ? encodeURIComponent(normalizedCampaignId) : normalizedCampaignId;
  const path = `/api/campaigns/${campaignIdSegment}/generate-draft`;
  return normalizedBaseUrl ? `${normalizedBaseUrl}${path}` : path;
}

function buildCreateCampaignEndpoint(baseUrl) {
  const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
  return normalizedBaseUrl ? `${normalizedBaseUrl}/api/campaigns` : "/api/campaigns";
}

function parseSSEBlock(rawBlock) {
  const lines = rawBlock.split("\n");
  let eventName = "message";
  const dataLines = [];

  for (const line of lines) {
    if (line.startsWith("event:")) {
      eventName = line.slice(6).trim();
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  const rawData = dataLines.join("\n");
  if (!rawData) {
    return { eventName, data: null };
  }

  try {
    return { eventName, data: JSON.parse(rawData) };
  } catch {
    return { eventName, data: rawData };
  }
}

async function consumeSSEStream(reader, onEvent) {
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done }).replaceAll("\r", "");

    let delimiterIndex = buffer.indexOf("\n\n");
    while (delimiterIndex >= 0) {
      const block = buffer.slice(0, delimiterIndex).trim();
      buffer = buffer.slice(delimiterIndex + 2);

      if (block) {
        const parsedEvent = parseSSEBlock(block);
        onEvent(parsedEvent);
      }
      delimiterIndex = buffer.indexOf("\n\n");
    }

    if (done) {
      break;
    }
  }

  const rest = buffer.trim();
  if (rest) {
    onEvent(parseSSEBlock(rest));
  }
}

function App() {
  const [activeTab, setActiveTab] = useState("create");

  // Calendar
  const [calendarDate, setCalendarDate] = useState(() => new Date(2026, 4, 1)); // May 2026
  const [selectedCard, setSelectedCard] = useState(null);
  const autoNavigatedRef = useRef(false);

  // Platform picker
  const [platforms, setPlatforms] = useState([]);
  const [platformsLoading, setPlatformsLoading] = useState(false);
  const [platformsError, setPlatformsError] = useState("");
  // { [platformId]: string[] } — selected post-type keys per platform
  const [selectedPlatforms, setSelectedPlatforms] = useState({
    "AXqWG7U2qnpt": ["text-post", "newsletter"],
    "zBU1zqVICGfk": ["text-post"],
  });

  const [createForm, setCreateForm] = useState(initialCreateForm);
  const [createError, setCreateError] = useState("");
  const [createSuccess, setCreateSuccess] = useState("");
  const [isCreating, setIsCreating] = useState(false);

  const [draftForm, setDraftForm] = useState(initialDraftForm);
  const [cards, setCards] = useState([]);
  const [draftError, setDraftError] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const abortRef = useRef(null);
  const counterRef = useRef(0);

  useEffect(() => {
    return () => {
      if (abortRef.current) {
        abortRef.current.abort();
      }
    };
  }, []);

  // Auto-navigate calendar to the month of the first arriving post
  useEffect(() => {
    if (autoNavigatedRef.current) return;
    const first = cards.find((c) => c.kind === "post" && c.payload?.post?.publishDate);
    if (!first) return;
    const d = new Date(first.payload.post.publishDate);
    if (!isNaN(d.getTime())) {
      setCalendarDate(new Date(d.getFullYear(), d.getMonth(), 1));
      autoNavigatedRef.current = true;
    }
  }, [cards]);

  const postsByDay = useMemo(() => {
    const map = {};
    for (const card of cards) {
      if (card.kind !== "post") continue;
      const raw = card.payload?.post?.publishDate;
      if (!raw) continue;
      const d = new Date(raw);
      if (isNaN(d.getTime())) continue;
      const key = calendarKey(d);
      (map[key] ??= []).push(card);
    }
    return map;
  }, [cards]);

  const systemCards = useMemo(() => cards.filter((c) => c.kind !== "post"), [cards]);

  const draftEndpointPreview = useMemo(() => {
    const id = draftForm.campaignId.trim() || "{{campaignId}}";
    return buildGenerateDraftEndpoint(draftForm.baseUrl, id, false);
  }, [draftForm.baseUrl, draftForm.campaignId]);

  const inputClass =
    "w-full rounded-[3px] border border-slate-300 bg-white px-2.5 py-1.5 text-xs text-slate-800 outline-none transition focus:border-slate-400 focus:ring-1 focus:ring-slate-200";
  const labelClass = "mb-1 block text-[10px] font-semibold uppercase tracking-[0.08em] text-slate-600";

  const setCreateField = (field) => (event) => {
    const value = event.target.type === "checkbox" ? event.target.checked : event.target.value;
    setCreateForm((prev) => ({ ...prev, [field]: value }));
  };

  const setDraftField = (field) => (event) => {
    setDraftForm((prev) => ({ ...prev, [field]: event.target.value }));
  };

  const appendCard = (card) => {
    counterRef.current += 1;
    setCards((prev) => [
      ...prev,
      {
        id: counterRef.current,
        receivedAt: new Date().toISOString(),
        ...card,
      },
    ]);
  };

  const handleFetchPlatforms = async () => {
    if (platformsLoading) return;
    setPlatformsError("");
    setPlatformsLoading(true);
    try {
      if (createForm.cs_session.trim()) {
        document.cookie = `cs_session=${encodeURIComponent(createForm.cs_session.trim())}; path=/; SameSite=Lax`;
      }
      const baseUrl = draftForm.baseUrl.trim().replace(/\/+$/, "");
      const url = baseUrl ? `${baseUrl}/api/platforms` : "/api/platforms";
      const res = await fetch(url, { credentials: "include" });
      if (!res.ok) throw new Error(`Failed to load platforms (${res.status})`);
      const data = await res.json();
      setPlatforms(data);
    } catch (e) {
      setPlatformsError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setPlatformsLoading(false);
    }
  };

  const togglePlatform = (platformId) => {
    setSelectedPlatforms((prev) => {
      if (prev[platformId]) {
        const next = { ...prev };
        delete next[platformId];
        return next;
      }
      return { ...prev, [platformId]: [] };
    });
  };

  const togglePostType = (platformId, postTypeKey) => {
    setSelectedPlatforms((prev) => {
      const current = prev[platformId] ?? [];
      const next = current.includes(postTypeKey)
        ? current.filter((k) => k !== postTypeKey)
        : [...current, postTypeKey];
      return { ...prev, [platformId]: next };
    });
  };

  const handleCreateCampaign = async (event) => {
    event.preventDefault();
    if (isCreating) return;

    setCreateError("");
    setCreateSuccess("");

    const baseUrl = draftForm.baseUrl.trim().replace(/\/+$/, "");
    const endpoint = buildCreateCampaignEndpoint(baseUrl);

    try {
      const payload = {
        name: createForm.name.trim(),
        description: createForm.description.trim(),
        target_persona: createForm.target_persona.trim(),
        key_messages: createForm.key_messages.trim(),
        tone_guidelines: createForm.tone_guidelines.trim(),
        use_pieces: createForm.use_pieces,
        pieces_ids: normalizeList(createForm.pieces_ids),
        target_platforms: Object.entries(selectedPlatforms)
          .filter(([, types]) => types.length > 0)
          .map(([id, post_types]) => ({ id, post_types })),
        objective: createForm.objective,
        status: createForm.status,
        start_date: toISOStringOrNull(createForm.start_date),
        end_date: toISOStringOrNull(createForm.end_date),
        estimated_post_count: parseOptionalNumber(createForm.estimated_post_count, "estimated_post_count"),
        budget: parseOptionalNumber(createForm.budget, "budget"),
        currency: createForm.currency.trim(),
        language: createForm.language.trim(),
        tag_ids: normalizeList(createForm.tag_ids),
      };

      if (createForm.cs_session.trim()) {
        document.cookie = `cs_session=${encodeURIComponent(createForm.cs_session.trim())}; path=/; SameSite=Lax`;
      }

      setIsCreating(true);

      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(payload),
      });

      const body = await response.json();

      if (!response.ok) {
        throw new Error(`Request failed (${response.status}): ${JSON.stringify(body)}`);
      }

      const campaignId = body.id || "";
      setDraftForm((prev) => ({ ...prev, campaignId }));
      setCards([]);
      autoNavigatedRef.current = false;
      setCreateSuccess(`Campaign created — ID: ${campaignId}`);
    } catch (error) {
      setCreateError(error instanceof Error ? error.message : "Unknown error");
    } finally {
      setIsCreating(false);
    }
  };

  const handleGenerateDraft = async (event) => {
    event.preventDefault();

    if (isStreaming) {
      return;
    }

    const baseUrl = draftForm.baseUrl.trim().replace(/\/+$/, "");
    const campaignId = draftForm.campaignId.trim();
    if (!campaignId) {
      setDraftError("campaignId is required");
      return;
    }
    const endpoint = buildGenerateDraftEndpoint(baseUrl, campaignId);

    try {
      const controller = new AbortController();
      abortRef.current = controller;

      setCards([]);
      setDraftError("");
      setIsStreaming(true);

      const response = await fetch(endpoint, {
        method: "POST",
        headers: {
          Accept: "text/event-stream",
          "Content-Type": "application/json",
        },
        credentials: "include",
        body: JSON.stringify({}),
        signal: controller.signal,
      });

      if (!response.ok) {
        const bodyText = await response.text();
        throw new Error(`Request failed (${response.status}): ${bodyText || response.statusText}`);
      }

      if (!response.body) {
        throw new Error("No response body stream was returned by the endpoint");
      }

      appendCard({
        kind: "request",
        title: "Request started",
        payload: { endpoint },
      });

      await consumeSSEStream(response.body.getReader(), ({ eventName, data }) => {
        if (eventName === "step") {
          appendCard({
            kind: "step",
            title: data?.step || "step",
            payload: data,
          });
          return;
        }

        if (eventName === "post") {
          appendCard({
            kind: "post",
            title: data?.post?.title || `Post #${(data?.index ?? 0) + 1}`,
            payload: data,
          });
          return;
        }

        if (eventName === "complete") {
          appendCard({
            kind: "complete",
            title: "Generation complete",
            payload: data,
          });
          return;
        }

        if (eventName === "error") {
          const message = data?.message || "Unknown stream error";
          setDraftError(message);
          appendCard({
            kind: "error",
            title: "Generation error",
            payload: data,
          });
          return;
        }

        appendCard({
          kind: "event",
          title: eventName,
          payload: data,
        });
      });
    } catch (error) {
      if (error.name === "AbortError") {
        appendCard({
          kind: "event",
          title: "Stream stopped",
          payload: { message: "Streaming was stopped manually." },
        });
        return;
      }

      const message = error instanceof Error ? error.message : "Unknown error";
      setDraftError(message);
      appendCard({
        kind: "error",
        title: "Client error",
        payload: { message },
      });
    } finally {
      setIsStreaming(false);
      abortRef.current = null;
    }
  };

  const handleStop = () => {
    if (abortRef.current) {
      abortRef.current.abort();
    }
  };

  const tabClass = (tab) =>
    `flex flex-1 flex-col items-center gap-1.5 px-4 py-4 text-[11px] font-semibold uppercase tracking-wider transition border-b-2 ${
      activeTab === tab
        ? "border-slate-700 text-slate-900 bg-white"
        : "border-transparent text-slate-400 hover:text-slate-600 hover:bg-slate-50"
    }`;

  return (
    <div className="h-screen overflow-hidden bg-slate-50 text-slate-800">
      <header className="border-b border-slate-200 bg-white px-6 py-3">
        <h1 className="text-sm font-semibold text-slate-600">CON-28 As a User, I want to generate an AI-augmented draft Content Plan for my Campaign</h1>
      </header>
      <main className="grid h-full grid-cols-1 divide-y divide-slate-200 lg:grid-cols-3 lg:divide-x lg:divide-y-0" style={{height: "calc(100% - 49px)"}}>

        {/* Left panel — tabbed */}
        <section className="flex h-full flex-col overflow-hidden">
          <div className="flex border-b border-slate-200 bg-slate-50">
            <button type="button" className={tabClass("create")} onClick={() => setActiveTab("create")}>
              <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 11l19-9-9 19-2-8-8-2z"/>
              </svg>
              Create Campaign
            </button>
            <button type="button" className={tabClass("draft")} onClick={() => setActiveTab("draft")}>
              <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12 20h9"/>
                <path d="M16.5 3.5a2.121 2.121 0 013 3L7 19l-4 1 1-4L16.5 3.5z"/>
              </svg>
              Generate Content Plan
            </button>
          </div>

          {/* Tab: Create Campaign */}
          {activeTab === "create" && (
          <div className="flex-1 overflow-y-auto p-6">
          <p className="text-xs text-slate-600">
            POST /api/campaigns — creates a new campaign and populates the Campaign ID field.
          </p>

          <form className="mt-4 space-y-3" onSubmit={handleCreateCampaign}>
            <div>
              <label className={labelClass} htmlFor="cs_session">
                cs_session cookie (optional)
              </label>
              <input
                id="cs_session"
                className={inputClass}
                value={createForm.cs_session}
                onChange={setCreateField("cs_session")}
                placeholder="paste cs_session value"
              />
            </div>

            <div>
              <label className={labelClass} htmlFor="create_name">
                name
              </label>
              <input id="create_name" className={inputClass} value={createForm.name} onChange={setCreateField("name")} />
            </div>

            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div>
                <label className={labelClass} htmlFor="create_objective">
                  objective
                </label>
                <select id="create_objective" className={inputClass} value={createForm.objective} onChange={setCreateField("objective")}>
                  <option value="awareness">awareness</option>
                  <option value="engagement">engagement</option>
                  <option value="conversion">conversion</option>
                  <option value="retention">retention</option>
                </select>
              </div>

              <div>
                <label className={labelClass} htmlFor="create_status">
                  status
                </label>
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
              <label className={labelClass} htmlFor="create_description">
                description
              </label>
              <textarea
                id="create_description"
                className={inputClass}
                rows={3}
                value={createForm.description}
                onChange={setCreateField("description")}
              />
            </div>

            <div>
              <label className={labelClass} htmlFor="create_target_persona">
                target_persona
              </label>
              <textarea
                id="create_target_persona"
                className={inputClass}
                rows={2}
                value={createForm.target_persona}
                onChange={setCreateField("target_persona")}
              />
            </div>

            <div>
              <label className={labelClass} htmlFor="create_key_messages">
                key_messages
              </label>
              <textarea
                id="create_key_messages"
                className={inputClass}
                rows={2}
                value={createForm.key_messages}
                onChange={setCreateField("key_messages")}
              />
            </div>

            <div>
              <label className={labelClass} htmlFor="create_tone_guidelines">
                tone_guidelines
              </label>
              <textarea
                id="create_tone_guidelines"
                className={inputClass}
                rows={2}
                value={createForm.tone_guidelines}
                onChange={setCreateField("tone_guidelines")}
              />
            </div>

            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div>
                <label className={labelClass} htmlFor="create_start_date">
                  start_date
                </label>
                <input
                  id="create_start_date"
                  type="date"
                  className={inputClass}
                  value={createForm.start_date}
                  onChange={setCreateField("start_date")}
                />
              </div>

              <div>
                <label className={labelClass} htmlFor="create_end_date">
                  end_date
                </label>
                <input
                  id="create_end_date"
                  type="date"
                  className={inputClass}
                  value={createForm.end_date}
                  onChange={setCreateField("end_date")}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div>
                <label className={labelClass} htmlFor="create_estimated_post_count">
                  estimated_post_count
                </label>
                <input
                  id="create_estimated_post_count"
                  className={inputClass}
                  value={createForm.estimated_post_count}
                  onChange={setCreateField("estimated_post_count")}
                />
              </div>

              <div>
                <label className={labelClass} htmlFor="create_budget">
                  budget
                </label>
                <input id="create_budget" className={inputClass} value={createForm.budget} onChange={setCreateField("budget")} />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div>
                <label className={labelClass} htmlFor="create_currency">
                  currency
                </label>
                <input id="create_currency" className={inputClass} value={createForm.currency} onChange={setCreateField("currency")} />
              </div>

              <div>
                <label className={labelClass} htmlFor="create_language">
                  language
                </label>
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
              <label className={labelClass} htmlFor="create_pieces_ids">
                pieces_ids (comma separated)
              </label>
              <input
                id="create_pieces_ids"
                className={inputClass}
                value={createForm.pieces_ids}
                onChange={setCreateField("pieces_ids")}
                placeholder="piece_a,piece_b"
              />
            </div>

            <div>
              <label className={labelClass} htmlFor="create_tag_ids">
                tag_ids (comma separated)
              </label>
              <input
                id="create_tag_ids"
                className={inputClass}
                value={createForm.tag_ids}
                onChange={setCreateField("tag_ids")}
                placeholder="tag_a,tag_b"
              />
            </div>

            {/* Platform picker */}
            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className={labelClass}>target platforms</span>
                <button
                  type="button"
                  onClick={handleFetchPlatforms}
                  disabled={platformsLoading}
                  className="rounded-[3px] border border-slate-200 bg-white px-2.5 py-1 text-[10px] font-semibold text-slate-600 transition hover:bg-slate-50 disabled:opacity-50"
                >
                  {platformsLoading ? "Loading…" : platforms.length ? "Refresh" : "Load Platforms"}
                </button>
              </div>

              {platformsError && (
                <p className="mb-2 rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">{platformsError}</p>
              )}

              {platforms.length === 0 ? (
                <p className="rounded-[3px] border border-dashed border-slate-200 px-3 py-4 text-center text-[11px] text-slate-400">
                  Click "Load Platforms" to fetch available platforms.
                </p>
              ) : (
                <div className="space-y-2">
                  {platforms.map((platform) => {
                    const cfg = platformConfig(platform.id);
                    const isEnabled = Boolean(selectedPlatforms[platform.id]);
                    const selectedTypes = selectedPlatforms[platform.id] ?? [];
                    const postTypeEntries = Object.entries(platform.post_types ?? {});

                    return (
                      <div key={platform.id} className={`rounded-[3px] border transition ${isEnabled ? "border-slate-300 bg-white" : "border-slate-200 bg-slate-50"}`}>
                        {/* Platform header row */}
                        <button
                          type="button"
                          onClick={() => togglePlatform(platform.id)}
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
                            <span className="ml-auto text-[10px] text-slate-400">{selectedTypes.length} type{selectedTypes.length !== 1 ? "s" : ""}</span>
                          )}
                        </button>

                        {/* Post type chips */}
                        {isEnabled && (
                          <div className="flex flex-wrap gap-1.5 border-t border-slate-100 px-3 pb-3 pt-2">
                            {postTypeEntries.map(([key, label]) => {
                              const active = selectedTypes.includes(key);
                              return (
                                <button
                                  key={key}
                                  type="button"
                                  onClick={() => togglePostType(platform.id, key)}
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

            {createError ? (
              <p className="rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                {createError}
              </p>
            ) : null}

            {createSuccess ? (
              <p className="rounded-[3px] border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-700">
                {createSuccess}
              </p>
            ) : null}

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
          )}

          {/* Tab: Generate Draft */}
          {activeTab === "draft" && (
          <div className="flex-1 overflow-y-auto p-6">
          <p className="text-xs text-slate-600">
            Stream content plan generation events for an existing campaign.
          </p>
          <p className="mt-4 break-all rounded-[3px] border border-slate-200 bg-slate-100 px-3 py-2 text-xs text-slate-700">
            POST {draftEndpointPreview}
          </p>

          <form className="mt-4 space-y-3" onSubmit={handleGenerateDraft}>
            <div>
              <label className={labelClass} htmlFor="baseUrl">
                baseUrl (optional)
              </label>
              <input
                id="baseUrl"
                className={inputClass}
                value={draftForm.baseUrl}
                onChange={setDraftField("baseUrl")}
                placeholder="leave empty to use /api dev proxy -> localhost:9002"
              />
            </div>

            <div>
              <label className={labelClass} htmlFor="campaignId">
                campaignId
              </label>
              <input
                id="campaignId"
                className={inputClass}
                value={draftForm.campaignId}
                onChange={setDraftField("campaignId")}
                placeholder="sqid campaign id (auto-filled on create)"
              />
            </div>

            {draftError ? (
              <p className="rounded-[3px] border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                {draftError}
              </p>
            ) : null}

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
          )}
        </section>

        {/* Panel 2 — Calendar + System Messages (spans 2 cols) */}
        <section className="flex h-full flex-col overflow-hidden lg:col-span-2">

          {/* Calendar */}
          <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-6">
            {/* Month nav */}
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-slate-900">
                {MONTH_NAMES[calendarDate.getMonth()]} {calendarDate.getFullYear()}
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

            {/* Day-of-week header */}
            <div className="grid grid-cols-7 border-l border-t border-slate-200">
              {DAY_NAMES.map((d) => (
                <div key={d} className="border-b border-r border-slate-200 bg-slate-50 px-2 py-1.5 text-center text-[10px] font-semibold uppercase tracking-wider text-slate-400">
                  {d}
                </div>
              ))}

              {/* Leading empty cells */}
              {(() => {
                const year = calendarDate.getFullYear();
                const month = calendarDate.getMonth();
                const firstDow = new Date(year, month, 1).getDay();
                const daysInMonth = new Date(year, month + 1, 0).getDate();
                const today = new Date();
                const cells = [];

                for (let i = 0; i < firstDow; i++) {
                  cells.push(
                    <div key={`empty-${i}`} className="min-h-[120px] border-b border-r border-slate-200 bg-slate-50/40" />
                  );
                }

                for (let day = 1; day <= daysInMonth; day++) {
                  const cellDate = new Date(year, month, day);
                  const key = calendarKey(cellDate);
                  const posts = postsByDay[key] || [];
                  const isToday =
                    today.getFullYear() === year &&
                    today.getMonth() === month &&
                    today.getDate() === day;

                  cells.push(
                    <div key={day} className="min-h-[120px] border-b border-r border-slate-200 bg-white p-1.5">
                      <span className={`mb-1 flex h-5 w-5 items-center justify-center rounded-full text-[11px] font-semibold ${
                        isToday ? "bg-slate-800 text-white" : "text-slate-500"
                      }`}>
                        {day}
                      </span>
                      <div className="space-y-0.5">
                        {posts.slice(0, 4).map((card) => {
                          const pid = card.payload?.post?.platformId;
                          const cfg = platformConfig(pid);
                          return (
                            <button
                              key={card.id}
                              type="button"
                              onClick={() => setSelectedCard(card)}
                              className={`fade-in-card flex w-full items-start gap-1.5 rounded-[2px] px-2 py-1 text-left text-[11px] font-medium leading-snug transition ${cfg.chip}`}
                            >
                              <span className="mt-0.5 shrink-0">{cfg.icon}</span>
                              <span>{card.payload?.post?.title || `Post #${(card.payload?.index ?? 0) + 1}`}</span>
                            </button>
                          );
                        })}
                        {posts.length > 4 && (
                          <button
                            type="button"
                            onClick={() => setSelectedCard(posts[4])}
                            className="w-full text-left text-[10px] text-slate-400 hover:text-slate-600"
                          >
                            +{posts.length - 4} more
                          </button>
                        )}
                      </div>
                    </div>
                  );
                }

                return cells;
              })()}
            </div>
          </div>

          {/* System Messages strip */}
          <div className="border-t border-slate-200">
            <div className="flex items-center gap-2 bg-slate-50 px-4 py-2">
              <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-slate-400">
                <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
              </svg>
              <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">System Messages</span>
              <span className="ml-auto text-[10px] text-slate-400">{systemCards.length} events</span>
            </div>
            <div className="flex h-56 flex-col-reverse gap-1 overflow-y-auto bg-slate-900 px-3 py-2">
              {systemCards.length === 0 ? (
                <p className="text-[11px] text-slate-500 italic">No system events yet.</p>
              ) : (
                [...systemCards].reverse().map((card) => {
                  const accent =
                    card.kind === "error" ? "text-rose-400" :
                    card.kind === "complete" ? "text-emerald-400" :
                    card.kind === "request" ? "text-sky-400" :
                    card.kind === "step" ? "text-amber-400" :
                    "text-slate-400";
                  return (
                    <div key={card.id} className="flex items-start gap-2 font-mono text-[11px]">
                      <span className="shrink-0 text-slate-500">{new Date(card.receivedAt).toLocaleTimeString()}</span>
                      <span className={`shrink-0 font-semibold ${accent}`}>[{card.kind}]</span>
                      <span className="text-slate-300 truncate">{card.title}{card.payload?.message ? ` — ${card.payload.message}` : ""}{card.payload?.step ? ` — ${card.payload.step}` : ""}</span>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </section>

        {/* Post detail modal */}
        {selectedCard && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
            onClick={() => setSelectedCard(null)}
          >
            <div
              className="w-full max-w-lg rounded-[4px] border border-slate-200 bg-white shadow-xl"
              onClick={(e) => e.stopPropagation()}
            >
              <header className="flex items-center justify-between border-b border-slate-200 px-5 py-3">
                <h3 className="text-sm font-semibold text-slate-900">
                  {selectedCard.payload?.post?.title || `Post #${(selectedCard.payload?.index ?? 0) + 1}`}
                </h3>
                <button
                  type="button"
                  onClick={() => setSelectedCard(null)}
                  className="text-slate-400 hover:text-slate-600 text-lg leading-none"
                >
                  ×
                </button>
              </header>
              <div className="space-y-3 px-5 py-4 text-xs text-slate-700">
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Platform</p>
                    <div className="mt-1 flex items-center gap-1.5">
                      {(() => {
                        const cfg = platformConfig(selectedCard.payload?.post?.platformId);
                        return (
                          <span className={`inline-flex items-center gap-1.5 rounded-[3px] px-2 py-0.5 text-xs font-medium ${cfg.chip}`}>
                            {cfg.icon}
                            {cfg.label}
                          </span>
                        );
                      })()}
                    </div>
                  </div>
                  <div>
                    <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Content Type</p>
                    <p className="mt-0.5">{selectedCard.payload?.post?.contentType || "-"}</p>
                  </div>
                  <div>
                    <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Publish Date</p>
                    <p className="mt-0.5">{selectedCard.payload?.post?.publishDate || "-"}</p>
                  </div>
                  <div>
                    <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Index</p>
                    <p className="mt-0.5">{selectedCard.payload?.index ?? "-"}</p>
                  </div>
                </div>
                <div>
                  <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Body</p>
                  <p className="mt-1 max-h-64 overflow-y-auto whitespace-pre-wrap rounded-[3px] bg-slate-50 p-3 leading-relaxed">
                    {selectedCard.payload?.post?.body || "-"}
                  </p>
                </div>
                <div>
                  <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">Tone Notes</p>
                  <p className="mt-1 whitespace-pre-wrap rounded-[3px] bg-slate-50 p-3 leading-relaxed">
                    {selectedCard.payload?.post?.toneNotes || "-"}
                  </p>
                </div>
              </div>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

export default App;
