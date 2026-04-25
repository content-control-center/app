import { useCallback, useEffect, useState } from "react";

const SIDEBAR_KEY = "con42-sidebar-collapsed";
const POSTS_KEY = "con42-posts-collapsed";
const CHAT_KEY = "con42-chat-collapsed";

import { CampaignSidebar } from "./components/CampaignSidebar.jsx";
import { ChatPanel } from "./components/ChatPanel.jsx";
import { PostContent } from "./components/PostContent.jsx";
import { PostDetails } from "./components/PostDetails.jsx";
import { PostPicker } from "./components/PostPicker.jsx";
import {
  buildAssistantEndpoint,
  buildMessagesEndpoint,
  buildPostEndpoint,
  buildVersionsEndpoint,
} from "./lib/api.js";

function App() {
  const [baseUrl, setBaseUrl] = useState("");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem(SIDEBAR_KEY) === "true");
  const [postsCollapsed, setPostsCollapsed] = useState(() => localStorage.getItem(POSTS_KEY) === "true");
  const [chatCollapsed, setChatCollapsed] = useState(() => localStorage.getItem(CHAT_KEY) === "true");
  const [campaigns, setCampaigns] = useState([]);
  const [campaignsLoading, setCampaignsLoading] = useState(false);
  const [campaign, setCampaign] = useState(null);
  const [post, setPost] = useState(null);
  const [content, setContent] = useState("");
  // Individual Markdown fragments from content_delta events while the
  // assistant is streaming. Each fragment renders as its own span so the
  // React key preserves old chunks (no re-animation) while new chunks
  // fade in on mount. Cleared on complete — the editor then snaps back
  // to the authoritative BlockNote JSON.
  const [streamingChunks, setStreamingChunks] = useState([]);
  const [versions, setVersions] = useState([]);
  const [messages, setMessages] = useState([]);
  const [isLoading, setIsLoading] = useState(false);

  // Fetch campaigns at App level so it works regardless of sidebar collapse state
  useEffect(() => {
    let cancelled = false;

    async function load() {
      setCampaignsLoading(true);
      try {
        const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
        const res = await fetch(`${base}/api/campaigns`, { credentials: "include" });
        if (!res.ok) throw new Error(`${res.status}`);
        const data = await res.json();
        if (!cancelled) {
          const list = Array.isArray(data) ? data : [];
          setCampaigns(list);

          // Restore campaign and post from URL
          const params = new URLSearchParams(window.location.search);
          const urlCampaignId = params.get("campaignId");
          if (urlCampaignId) {
            const match = list.find((c) => c.id === urlCampaignId);
            if (match) setCampaign(match);
          }
          const urlPostId = params.get("postId");
          if (urlPostId) {
            const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
            try {
              const postRes = await fetch(`${base}/api/posts/${urlPostId}`, { credentials: "include" });
              if (postRes.ok) {
                const postData = await postRes.json();
                if (!cancelled) {
                  setPost(postData);
                  setContent(postData.content || "");
                  // Also fetch versions and messages
                  const [verRes, msgRes] = await Promise.all([
                    fetch(`${base}/api/posts/${urlPostId}/versions`, { credentials: "include" }),
                    fetch(`${base}/api/posts/${urlPostId}/messages`, { credentials: "include" }),
                  ]);
                  if (verRes.ok) {
                    const verData = await verRes.json();
                    if (!cancelled) setVersions(Array.isArray(verData) ? verData : []);
                  }
                  if (msgRes.ok) {
                    const msgData = await msgRes.json();
                    const list = Array.isArray(msgData) ? msgData : [];
                    if (!cancelled) setMessages(list.map(mapHistoryMessage));
                  }
                }
              }
            } catch {
              // silent
            }
          }
        }
      } catch {
        // silent
      } finally {
        if (!cancelled) setCampaignsLoading(false);
      }
    }

    load();
    return () => { cancelled = true; };
  }, [baseUrl]);

  const mapHistoryMessage = (m) => {
    const role = m.role === "model" ? "assistant" : m.role;
    if (role === "assistant") {
      try {
        const parsed = JSON.parse(m.content);
        return {
          role,
          text: parsed.explanation || m.content,
          action: parsed.action,
          saveVersion: parsed.saveVersion,
          versionNote: parsed.versionNote,
        };
      } catch {
        // not JSON, use as-is
      }
    }
    return { role, text: m.content };
  };

  const fetchMessages = useCallback(async (postId) => {
    try {
      const url = buildMessagesEndpoint(baseUrl, postId);
      const res = await fetch(url, { credentials: "include" });
      if (res.ok) {
        const data = await res.json();
        const list = Array.isArray(data) ? data : [];
        setMessages(list.map(mapHistoryMessage));
      }
    } catch {
      // silent
    }
  }, [baseUrl]);

  const fetchVersions = useCallback(async (postId) => {
    try {
      const url = buildVersionsEndpoint(baseUrl, postId);
      const res = await fetch(url, { credentials: "include" });
      if (res.ok) {
        const data = await res.json();
        setVersions(Array.isArray(data) ? data : []);
      }
    } catch {
      // silent
    }
  }, [baseUrl]);

  const refreshPost = useCallback(async (postId) => {
    try {
      const url = buildPostEndpoint(baseUrl, postId);
      const res = await fetch(url, { credentials: "include" });
      if (res.ok) {
        const data = await res.json();
        setPost(data);
        setContent(data.content || "");
      }
    } catch {
      // silent
    }
  }, [baseUrl]);

  const toggleSidebar = () => {
    setSidebarCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(SIDEBAR_KEY, String(next));
      return next;
    });
  };

  const togglePosts = () => {
    setPostsCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(POSTS_KEY, String(next));
      return next;
    });
  };

  const toggleChat = () => {
    setChatCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(CHAT_KEY, String(next));
      return next;
    });
  };

  const handleDeleteCampaign = async (id) => {
    try {
      const base = baseUrl ? baseUrl.replace(/\/+$/, "") : "";
      const res = await fetch(`${base}/api/campaigns/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error(`Failed (${res.status})`);
      setCampaigns((prev) => prev.filter((c) => c.id !== id));
      if (campaign?.id === id) {
        setCampaign(null);
        setPost(null);
        setContent("");
        setVersions([]);
        setMessages([]);
        const params = new URLSearchParams(window.location.search);
        params.delete("campaignId");
        params.delete("postId");
        window.history.replaceState(null, "", params.toString() ? `?${params.toString()}` : window.location.pathname);
      }
    } catch {
      // silent
    }
  };

  const handleSelectCampaign = (c) => {
    setCampaign(c);
    setPost(null);
    setContent("");
    setVersions([]);
    setMessages([]);
    const params = new URLSearchParams(window.location.search);
    params.set("campaignId", c.id);
    params.delete("postId");
    window.history.replaceState(null, "", `?${params.toString()}`);
  };

  const handleSelectPost = (p) => {
    setPost(p);
    setContent(p.content || "");
    setMessages([]);
    setVersions([]);
    fetchMessages(p.id);
    fetchVersions(p.id);
    refreshPost(p.id);
    const params = new URLSearchParams(window.location.search);
    params.set("postId", p.id);
    window.history.replaceState(null, "", `?${params.toString()}`);
  };

  const handleSend = async (instruction) => {
    if (!post) return;

    // Push the user message and a placeholder assistant bubble that the
    // stream will progressively fill in.
    setMessages((prev) => [
      ...prev,
      { role: "user", text: instruction },
      { role: "assistant", text: "", toolCalls: [], streaming: true },
    ]);
    setStreamingChunks([]);
    setIsLoading(true);

    // Update only the last message in the list — used by each SSE event
    // handler to mutate the in-flight assistant bubble in place.
    const updateLast = (patch) => {
      setMessages((prev) => {
        if (prev.length === 0) return prev;
        const next = prev.slice();
        const last = next[next.length - 1];
        next[next.length - 1] = typeof patch === "function" ? patch(last) : { ...last, ...patch };
        return next;
      });
    };

    try {
      const url = buildAssistantEndpoint(baseUrl, post.id);
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
        credentials: "include",
        body: JSON.stringify({ instruction }),
      });

      if (!res.ok || !res.body) {
        const body = res.body ? await res.text() : "no response body";
        throw new Error(`Request failed (${res.status}): ${body}`);
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = "";

      const handleEvent = (event, dataStr) => {
        let data;
        try { data = JSON.parse(dataStr); } catch { return; }
        switch (event) {
          case "explanation_delta":
            updateLast((m) => ({ ...m, text: (m.text || "") + (data.delta || "") }));
            break;
          case "content_delta":
            if (data.delta) setStreamingChunks((prev) => [...prev, data.delta]);
            break;
          case "tool_call":
            updateLast((m) => ({
              ...m,
              toolCalls: [
                ...(m.toolCalls || []),
                { name: data.name, ref: data.ref, input: data.input, status: "running" },
              ],
            }));
            break;
          case "tool_result":
            updateLast((m) => ({
              ...m,
              toolCalls: (m.toolCalls || []).map((tc) =>
                tc.ref === data.ref ? { ...tc, status: "done" } : tc,
              ),
            }));
            break;
          case "complete":
            updateLast((m) => ({
              ...m,
              text: data.explanation || m.text || "Done.",
              action: data.action,
              saveVersion: data.saveVersion,
              versionNote: data.versionNote,
              streaming: false,
            }));
            if (data.action === "edited" && data.updatedContent) {
              setContent(data.updatedContent);
            }
            setStreamingChunks([]);
            break;
          case "error":
            updateLast({ role: "error", text: data.message || "Assistant failed", streaming: false });
            setStreamingChunks([]);
            break;
          default:
            break;
        }
      };

      // Parse SSE frames: blocks separated by a blank line, each block has
      // `event: <name>` and `data: <json>` lines.
      const flushFrames = () => {
        let idx;
        while ((idx = buf.indexOf("\n\n")) >= 0) {
          const frame = buf.slice(0, idx);
          buf = buf.slice(idx + 2);
          let event = "", dataStr = "";
          for (const line of frame.split("\n")) {
            if (line.startsWith("event: ")) event = line.slice(7);
            else if (line.startsWith("data: ")) dataStr = line.slice(6);
          }
          if (event) handleEvent(event, dataStr);
        }
      };

      // eslint-disable-next-line no-constant-condition
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        flushFrames();
      }
      buf += decoder.decode();
      flushFrames();

      await Promise.all([fetchVersions(post.id), refreshPost(post.id)]);
    } catch (e) {
      updateLast({ role: "error", text: e.message, streaming: false });
      setStreamingChunks([]);
    } finally {
      setIsLoading(false);
    }
  };

  const handleCreateVersion = async () => {
    if (!post) return;
    try {
      const url = buildVersionsEndpoint(baseUrl, post.id);
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ note: "Manual save" }),
      });
      if (!res.ok) throw new Error(`Failed (${res.status})`);
      await fetchVersions(post.id);
    } catch (e) {
      setMessages((prev) => [...prev, { role: "error", text: e.message }]);
    }
  };

  return (
    <div className="h-screen overflow-hidden bg-slate-50 text-slate-800">
      <header className="border-b border-slate-200 bg-white px-6 py-3 flex items-center gap-4">
        <h1 className="text-sm font-semibold text-slate-600 flex-1">
          CON-42 As a User, I want to use a Post Assistant to enhance post content
        </h1>
        <input
          type="text"
          placeholder="Base URL (blank = same origin)"
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          className="w-56 rounded border border-slate-300 bg-slate-50 px-2.5 py-1 text-xs focus:border-slate-500 focus:outline-none"
        />
      </header>

      <main
        className="flex h-full divide-x divide-slate-200"
        style={{ height: "calc(100% - 49px)" }}
      >
        {/* Campaigns sidebar — collapsible */}
        <section
          className="flex h-full flex-col overflow-hidden transition-[width] duration-200"
          style={{ width: sidebarCollapsed ? "40px" : "15%", minWidth: sidebarCollapsed ? "40px" : "160px" }}
        >
          <div className="flex shrink-0 items-center border-b border-slate-200 bg-slate-50 px-2 py-2.5">
            <button
              type="button"
              onClick={toggleSidebar}
              className="rounded p-1 text-slate-400 hover:bg-slate-200 hover:text-slate-600 transition"
              title={sidebarCollapsed ? "Expand campaigns" : "Collapse campaigns"}
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                {sidebarCollapsed ? (
                  <>
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M9 3v18" />
                    <path d="m14 9 3 3-3 3" />
                  </>
                ) : (
                  <>
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M9 3v18" />
                    <path d="m16 15-3-3 3-3" />
                  </>
                )}
              </svg>
            </button>
            {!sidebarCollapsed && (
              <h2 className="ml-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-400">Campaigns</h2>
            )}
          </div>
          {sidebarCollapsed ? (
            <div className="flex flex-1 items-start justify-center pt-4">
              <span
                className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 whitespace-nowrap"
                style={{ writingMode: "vertical-rl" }}
              >
                Campaigns
              </span>
            </div>
          ) : (
            <CampaignSidebar campaigns={campaigns} loading={campaignsLoading} selectedId={campaign?.id} onSelect={handleSelectCampaign} onDelete={handleDeleteCampaign} />
          )}
        </section>

        {/* Posts + Description — collapsible */}
        <section
          className="flex h-full flex-col overflow-hidden transition-[width] duration-200"
          style={{ width: postsCollapsed ? "40px" : "15%", minWidth: postsCollapsed ? "40px" : "160px" }}
        >
          <div className="flex shrink-0 items-center border-b border-slate-200 bg-slate-50 px-2 py-2.5">
            <button
              type="button"
              onClick={togglePosts}
              className="rounded p-1 text-slate-400 hover:bg-slate-200 hover:text-slate-600 transition"
              title={postsCollapsed ? "Expand posts" : "Collapse posts"}
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                {postsCollapsed ? (
                  <>
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M9 3v18" />
                    <path d="m14 9 3 3-3 3" />
                  </>
                ) : (
                  <>
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M9 3v18" />
                    <path d="m16 15-3-3 3-3" />
                  </>
                )}
              </svg>
            </button>
            {!postsCollapsed && (
              <h2 className="ml-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-400">Posts</h2>
            )}
          </div>
          {postsCollapsed ? (
            <div className="flex flex-1 items-start justify-center pt-4">
              <span
                className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 whitespace-nowrap"
                style={{ writingMode: "vertical-rl" }}
              >
                Posts
              </span>
            </div>
          ) : (
            <>
              <PostPicker
                baseUrl={baseUrl}
                campaignId={campaign?.id}
                selectedId={post?.id}
                onSelect={handleSelectPost}
              />
            </>
          )}
        </section>

        {/* Post Details */}
        <section className="flex h-full flex-col overflow-hidden" style={{ width: "25%", minWidth: "220px" }}>
          <PostDetails
            post={post}
            versions={versions}
            onCreateVersion={handleCreateVersion}
            baseUrl={baseUrl}
            onPostUpdated={() => post && refreshPost(post.id)}
          />
        </section>

        {/* Post Content */}
        <section className="flex h-full flex-1 flex-col overflow-hidden" style={{ minWidth: "200px" }}>
          <PostContent post={post} content={content} streamingChunks={streamingChunks} />
        </section>

        {/* Assistant — collapsible to the right */}
        <section
          className="flex h-full min-h-0 shrink-0 flex-col overflow-hidden transition-[width] duration-200"
          style={{ width: chatCollapsed ? "40px" : "30%", minWidth: chatCollapsed ? "40px" : "280px" }}
        >
          <div className="flex shrink-0 items-center border-b border-slate-200 bg-slate-50 px-2 py-2.5">
            <button
              type="button"
              onClick={toggleChat}
              className="rounded p-1 text-slate-400 hover:bg-slate-200 hover:text-slate-600 transition"
              title={chatCollapsed ? "Expand assistant" : "Collapse assistant"}
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                {chatCollapsed ? (
                  <>
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M15 3v18" />
                    <path d="m10 15-3-3 3-3" />
                  </>
                ) : (
                  <>
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <path d="M15 3v18" />
                    <path d="m8 9 3 3-3 3" />
                  </>
                )}
              </svg>
            </button>
            {!chatCollapsed && (
              <h2 className="ml-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-400">Assistant</h2>
            )}
          </div>
          {chatCollapsed ? (
            <div className="flex flex-1 items-start justify-center pt-4">
              <span
                className="text-[10px] font-semibold uppercase tracking-wider text-slate-400 whitespace-nowrap"
                style={{ writingMode: "vertical-rl" }}
              >
                Assistant
              </span>
            </div>
          ) : (
            <ChatPanel messages={messages} onSend={handleSend} isLoading={isLoading} />
          )}
        </section>
      </main>
    </div>
  );
}

export default App;
