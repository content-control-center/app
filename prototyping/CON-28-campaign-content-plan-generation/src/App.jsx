import { useEffect, useMemo, useRef, useState } from "react";

import { Calendar } from "./components/Calendar.jsx";
import { CreateCampaignTab } from "./components/CreateCampaignTab.jsx";
import { GenerateDraftTab } from "./components/GenerateDraftTab.jsx";
import { PostDetailModal } from "./components/PostDetailModal.jsx";
import { SystemMessages } from "./components/SystemMessages.jsx";
import {
  buildCampaignTypesEndpoint,
  buildCreateCampaignEndpoint,
  buildGenerateDraftEndpoint,
  initialCreateForm,
  initialDraftForm,
  normalizeList,
  parseOptionalNumber,
  toISOStringOrNull,
} from "./lib/api.js";
import { calendarKey } from "./lib/calendar.js";
import { consumeSSEStream } from "./lib/sse.js";

function App() {
  const [activeTab, setActiveTab] = useState("create");

  // Calendar
  const [calendarDate, setCalendarDate] = useState(() => new Date(2026, 4, 1)); // May 2026
  const [selectedCard, setSelectedCard] = useState(null);
  const [dragOverKey, setDragOverKey] = useState(null);

  // Campaign types
  const [campaignTypes, setCampaignTypes] = useState([]);
  const [campaignTypesLoading, setCampaignTypesLoading] = useState(false);
  const [campaignTypesError, setCampaignTypesError] = useState("");

  // Platform picker
  const [platforms, setPlatforms] = useState([]);
  const [platformsLoading, setPlatformsLoading] = useState(false);
  const [platformsError, setPlatformsError] = useState("");
  const [selectedPlatforms, setSelectedPlatforms] = useState({
    "AXqWG7U2qnpt": ["text-post", "newsletter"],
    "zBU1zqVICGfk": ["text-post"],
  });

  const [createForm, setCreateForm] = useState(initialCreateForm);
  const [createError, setCreateError] = useState("");
  const [createSuccess, setCreateSuccess] = useState("");
  const [isCreating, setIsCreating] = useState(false);

  const [selectedPhase, setSelectedPhase] = useState(null);

  const activePhases = useMemo(() => {
    const type = campaignTypes.find((t) => t.id === createForm.campaign_type_id);
    return type?.phases
      ? [...type.phases].sort((a, b) => a.sequence - b.sequence)
      : [];
  }, [campaignTypes, createForm.campaign_type_id]);

  const [draftForm, setDraftForm] = useState(initialDraftForm);
  const [cards, setCards] = useState([]);
  const [expectedPostCount, setExpectedPostCount] = useState(0);
  const [draftError, setDraftError] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const abortRef = useRef(null);
  const counterRef = useRef(0);

  useEffect(() => {
    return () => { abortRef.current?.abort(); };
  }, []);

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
  const receivedPostCount = useMemo(
    () => cards.reduce((n, c) => (c.kind === "post" ? n + 1 : n), 0),
    [cards]
  );

  const draftEndpointPreview = useMemo(() => {
    const id = draftForm.campaignId.trim() || "{{campaignId}}";
    return buildGenerateDraftEndpoint(draftForm.baseUrl, id, false);
  }, [draftForm.baseUrl, draftForm.campaignId]);

  const setCreateField = (field) => (event) => {
    const value = event.target.type === "checkbox" ? event.target.checked : event.target.value;
    setCreateForm((prev) => ({ ...prev, [field]: value }));
  };

  const setDraftField = (field) => (event) => {
    setDraftForm((prev) => ({ ...prev, [field]: event.target.value }));
  };

  const moveCard = (cardId, newDate) => {
    setCards((prev) =>
      prev.map((c) =>
        c.id === cardId
          ? { ...c, payload: { ...c.payload, post: { ...c.payload.post, publishDate: newDate.toISOString() } } }
          : c
      )
    );
  };

  // appendCard pushes onto the rolling event log. Post events from the
  // backend are now batched and parallel — under CON-67 the same global
  // slot index could in principle re-arrive (defensive against retries),
  // so we replace any existing post card carrying the same payload.index
  // rather than appending a duplicate. Stream-arrival order is no longer
  // monotonic by index; the calendar layout is naturally robust to this
  // because posts are placed by publishDate.
  const appendCard = (card) => {
    if (card.kind === "post" && typeof card.payload?.index === "number") {
      const idx = card.payload.index;
      setCards((prev) => {
        const existing = prev.findIndex(
          (c) => c.kind === "post" && c.payload?.index === idx
        );
        if (existing >= 0) {
          const next = prev.slice();
          next[existing] = { ...prev[existing], payload: card.payload };
          return next;
        }
        counterRef.current += 1;
        return [
          ...prev,
          { id: counterRef.current, receivedAt: new Date().toISOString(), ...card },
        ];
      });
      return;
    }
    counterRef.current += 1;
    setCards((prev) => [
      ...prev,
      { id: counterRef.current, receivedAt: new Date().toISOString(), ...card },
    ]);
  };

  const handleFetchCampaignTypes = async () => {
    if (campaignTypesLoading) return;
    setCampaignTypesError("");
    setCampaignTypesLoading(true);
    try {
      const baseUrl = draftForm.baseUrl.trim().replace(/\/+$/, "");
      const url = buildCampaignTypesEndpoint(baseUrl);
      const res = await fetch(url, { credentials: "include" });
      if (!res.ok) throw new Error(`Failed to load campaign types (${res.status})`);
      const types = await res.json();
      setCampaignTypes(types);
      if (types.length > 0 && !createForm.campaign_type_id) {
        setCreateForm((prev) => ({ ...prev, campaign_type_id: types[0].id }));
      }
    } catch (e) {
      setCampaignTypesError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setCampaignTypesLoading(false);
    }
  };

  const handleFetchPlatforms = async () => {
    if (platformsLoading) return;
    setPlatformsError("");
    setPlatformsLoading(true);
    try {
      const baseUrl = draftForm.baseUrl.trim().replace(/\/+$/, "");
      const url = baseUrl ? `${baseUrl}/api/platforms` : "/api/platforms";
      const res = await fetch(url, { credentials: "include" });
      if (!res.ok) throw new Error(`Failed to load platforms (${res.status})`);
      setPlatforms(await res.json());
    } catch (e) {
      setPlatformsError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setPlatformsLoading(false);
    }
  };

  // Auto-fetch on mount
  useEffect(() => {
    handleFetchCampaignTypes();
    handleFetchPlatforms();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
        use_assets: createForm.use_assets,
        asset_ids: normalizeList(createForm.asset_ids),
        target_platforms: Object.entries(selectedPlatforms)
          .filter(([, types]) => types.length > 0)
          .map(([id, post_types]) => ({ id, post_types })),
        campaign_type_id: createForm.campaign_type_id.trim(),
        status: createForm.status,
        start_date: toISOStringOrNull(createForm.start_date),
        end_date: toISOStringOrNull(createForm.end_date),
        estimated_post_count: parseOptionalNumber(createForm.estimated_post_count, "estimated_post_count"),
        budget: parseOptionalNumber(createForm.budget, "budget"),
        currency: createForm.currency.trim(),
        language: createForm.language.trim(),
        tag_ids: normalizeList(createForm.tag_ids),
      };

      setIsCreating(true);
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(payload),
      });
      const body = await response.json();
      if (!response.ok) throw new Error(`Request failed (${response.status}): ${JSON.stringify(body)}`);

      const campaignId = body.id || "";
      setDraftForm((prev) => ({ ...prev, campaignId }));
      setCards([]);
      // Pre-position the calendar at the campaign's start month rather than
      // following the first post arrival. Under parallel batching the first
      // arriving post can be from any batch (i.e. any month), so navigating
      // to it would jump the calendar somewhere unhelpful.
      const startD = new Date(createForm.start_date);
      if (!Number.isNaN(startD.getTime())) {
        setCalendarDate(new Date(startD.getFullYear(), startD.getMonth(), 1));
      }
      // Pre-seed expected count so the progress indicator works from the
      // very first post arrival rather than only after `complete`.
      const expected = parseOptionalNumber(
        createForm.estimated_post_count,
        "estimated_post_count"
      );
      setExpectedPostCount(typeof expected === "number" ? expected : 0);
      setCreateSuccess(`Campaign created — ID: ${campaignId}`);
    } catch (error) {
      setCreateError(error instanceof Error ? error.message : "Unknown error");
    } finally {
      setIsCreating(false);
    }
  };

  const handleGenerateDraft = async (event) => {
    event.preventDefault();
    if (isStreaming) return;

    const baseUrl = draftForm.baseUrl.trim().replace(/\/+$/, "");
    const campaignId = draftForm.campaignId.trim();
    if (!campaignId) { setDraftError("campaignId is required"); return; }
    const endpoint = buildGenerateDraftEndpoint(baseUrl, campaignId);

    try {
      const controller = new AbortController();
      abortRef.current = controller;
      setCards([]);
      setSelectedPhase(null);
      setDraftError("");
      setIsStreaming(true);

      const response = await fetch(endpoint, {
        method: "POST",
        headers: { Accept: "text/event-stream", "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({}),
        signal: controller.signal,
      });

      if (!response.ok) {
        const bodyText = await response.text();
        throw new Error(`Request failed (${response.status}): ${bodyText || response.statusText}`);
      }
      if (!response.body) throw new Error("No response body stream was returned by the endpoint");

      appendCard({ kind: "request", title: "Request started", payload: { endpoint } });

      await consumeSSEStream(response.body.getReader(), ({ eventName, data }) => {
        if (eventName === "step") {
          appendCard({ kind: "step", title: data?.step || "step", payload: data });
        } else if (eventName === "post") {
          appendCard({ kind: "post", title: data?.post?.title || `Post #${(data?.index ?? 0) + 1}`, payload: data });
        } else if (eventName === "complete") {
          appendCard({ kind: "complete", title: "Generation complete", payload: data });
          // Lock in the authoritative final count so the progress indicator
          // reads "120 / 120" rather than "120 / 100" if the backend yielded
          // more (or fewer, on partial success) than the original estimate.
          if (Array.isArray(data?.posts)) {
            setExpectedPostCount(data.posts.length);
          }
          // Each warning becomes its own system card so partial-batch
          // failures (CON-67: backend aggregates failed batches into the
          // complete payload's warnings array) are visible without the
          // user having to expand the complete card.
          if (Array.isArray(data?.warnings)) {
            for (const w of data.warnings) {
              appendCard({ kind: "warning", title: "Warning", payload: { message: w } });
            }
          }
        } else if (eventName === "error") {
          const message = data?.message || "Unknown stream error";
          setDraftError(message);
          appendCard({ kind: "error", title: "Generation error", payload: data });
        } else {
          appendCard({ kind: "event", title: eventName, payload: data });
        }
      });
    } catch (error) {
      if (error.name === "AbortError") {
        appendCard({ kind: "event", title: "Stream stopped", payload: { message: "Streaming was stopped manually." } });
        return;
      }
      const message = error instanceof Error ? error.message : "Unknown error";
      setDraftError(message);
      appendCard({ kind: "error", title: "Client error", payload: { message } });
    } finally {
      setIsStreaming(false);
      abortRef.current = null;
    }
  };

  const handleStop = () => { abortRef.current?.abort(); };

  const tabClass = (tab) =>
    `flex flex-1 flex-col items-center gap-1.5 px-4 py-4 text-[11px] font-semibold uppercase tracking-wider transition border-b-2 ${
      activeTab === tab
        ? "border-slate-700 text-slate-900 bg-white"
        : "border-transparent text-slate-400 hover:text-slate-600 hover:bg-slate-50"
    }`;

  return (
    <div className="h-screen overflow-hidden bg-slate-50 text-slate-800">
      <header className="border-b border-slate-200 bg-white px-6 py-3">
        <h1 className="text-sm font-semibold text-slate-600">
          CON-28 As a User, I want to generate an AI-augmented draft Content Plan for my Campaign
        </h1>
      </header>

      <main
        className="grid h-full grid-cols-1 divide-y divide-slate-200 lg:grid-cols-3 lg:divide-x lg:divide-y-0"
        style={{ height: "calc(100% - 49px)" }}
      >
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

          {activeTab === "create" && (
            <CreateCampaignTab
              createForm={createForm}
              setCreateField={setCreateField}
              handleCreateCampaign={handleCreateCampaign}
              isCreating={isCreating}
              createError={createError}
              createSuccess={createSuccess}
              campaignTypes={campaignTypes}
              campaignTypesLoading={campaignTypesLoading}
              campaignTypesError={campaignTypesError}
              platforms={platforms}
              platformsLoading={platformsLoading}
              platformsError={platformsError}
              selectedPlatforms={selectedPlatforms}
              onTogglePlatform={togglePlatform}
              onTogglePostType={togglePostType}
            />
          )}

          {activeTab === "draft" && (
            <GenerateDraftTab
              draftForm={draftForm}
              setDraftField={setDraftField}
              handleGenerateDraft={handleGenerateDraft}
              isStreaming={isStreaming}
              handleStop={handleStop}
              draftError={draftError}
              draftEndpointPreview={draftEndpointPreview}
            />
          )}
        </section>

        {/* Right panel — Calendar + System Messages */}
        <section className="flex h-full flex-col overflow-hidden lg:col-span-2">
          <Calendar
            calendarDate={calendarDate}
            setCalendarDate={setCalendarDate}
            postsByDay={postsByDay}
            dragOverKey={dragOverKey}
            setDragOverKey={setDragOverKey}
            onDrop={moveCard}
            onCardClick={setSelectedCard}
            phases={activePhases}
            selectedPhase={selectedPhase}
            onSelectPhase={(id) => setSelectedPhase((prev) => (prev === id ? null : id))}
          />
          <SystemMessages
            systemCards={systemCards}
            receivedPostCount={receivedPostCount}
            expectedPostCount={expectedPostCount}
            isStreaming={isStreaming}
          />
        </section>
      </main>

      <PostDetailModal card={selectedCard} onClose={() => setSelectedCard(null)} campaignTypes={campaignTypes} />
    </div>
  );
}

export default App;
