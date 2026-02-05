"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { clearToken, getToken } from "@/lib/auth";

const PAGE_SIZE = 20;
const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";
type Me = { id: number; email: string; username: string };

type JobStatus = "queued" | "running" | "succeeded" | "failed";

type Job = {
  id: string;
  session_id: string;
  status: JobStatus;
  result_message_id: number | null;
  error: string | null;
  created_at: string;
  updated_at: string;
};

type ChatMessage = {
  id: number;
  session_id: string;
  role: "user" | "assistant" | "system";
  content: string;
  created_at: string;
};

type ListMessagesResp = {
  messages: ChatMessage[];
  next_before_id: number | null;
};

export default function ChatPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const sidFromUrl = searchParams.get("session_id");

  const [me, setMe] = useState<Me | null>(null);

  const [sessionId, setSessionId] = useState<string | null>(null);

  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [nextBeforeId, setNextBeforeId] = useState<number | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [responseMode, setResponseMode] = useState<"full" | "stream">("full");
  const [streamingText, setStreamingText] = useState("");
  const [streaming, setStreaming] = useState(false);

  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState<string | null>(null);

  const streamAbort = useRef<AbortController | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const authed = useMemo(() => Boolean(getToken()), []);

  function mergeUniqueById(prev: ChatMessage[], incoming: ChatMessage[]) {
    const map = new Map<number, ChatMessage>();
    for (const m of prev) map.set(m.id, m);
    for (const m of incoming) map.set(m.id, m);

    // Sort: oldest first so latest appears at bottom
    return Array.from(map.values()).sort((a, b) => a.id - b.id);
  }

  async function fetchMessages(opts: {
    sid: string;
    beforeId?: number;
    replace?: boolean;
  }) {
    const { sid, beforeId, replace } = opts;

    const params = new URLSearchParams();
    params.set("limit", String(PAGE_SIZE));
    if (beforeId) params.set("before_id", String(beforeId));

    const qs = `?${params.toString()}`;
    const data = await apiFetch<ListMessagesResp>(
      `/chat/sessions/${sid}/messages${qs}`,
      {
        auth: true,
      },
    );

    setNextBeforeId(data.next_before_id ?? null);

    const ordered = (data.messages ?? []).slice().sort((a, b) => a.id - b.id);
    if (replace) {
      setMessages(ordered);
    } else {
      setMessages((prev) => mergeUniqueById(prev, ordered));
    }
  }

  async function refreshMessages(sid: string) {
    setRefreshing(true);
    try {
      await fetchMessages({ sid, replace: true });
    } finally {
      setRefreshing(false);
    }
  }

  async function loadOlderMessages() {
    if (!sessionId) return;
    if (!nextBeforeId) return;

    setLoadingOlder(true);
    try {
      await fetchMessages({
        sid: sessionId,
        beforeId: nextBeforeId,
        replace: false,
      });
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Failed to load older messages",
      );
    } finally {
      setLoadingOlder(false);
    }
  }

  useEffect(() => {
    if (!authed) {
      router.replace("/login");
      return;
    }
    apiFetch<Me>("/me", { auth: true })
      .then(setMe)
      .catch((e) => setError(e?.message ?? "Failed to load /me"));
  }, [authed, router]);

  // Use session_id from URL (so refresh keeps the same session),
  // otherwise create a new session and persist it into the URL.
  useEffect(() => {
    if (!authed) return;

    if (!sidFromUrl) {
      setSessionId(null);
      setMessages([]);
      setNextBeforeId(null);
      setJob(null);
      return;
    }

    setSessionId(sidFromUrl);
    refreshMessages(sidFromUrl).catch((e) =>
      setError(e?.message ?? "Failed to load messages"),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authed, sidFromUrl]);

  useEffect(() => {
    return () => {
      if (streamAbort.current) streamAbort.current.abort();
    };
  }, []);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, streamingText, streaming]);

  async function sendFullMessage(sid: string, msg: string) {
    await apiFetch<{ reply: string; message_id: number }>("/chat/messages", {
      method: "POST",
      auth: true,
      body: JSON.stringify({ session_id: sid, message: msg }),
    });
    await refreshMessages(sid);
  }

  async function sendStreamMessage(sid: string, msg: string) {
    if (!API_BASE_URL) {
      throw new Error("Missing NEXT_PUBLIC_API_BASE_URL");
    }
    const token = getToken();
    if (!token) throw new Error("Not authenticated");

    if (streamAbort.current) streamAbort.current.abort();
    const controller = new AbortController();
    streamAbort.current = controller;

    const res = await fetch(`${API_BASE_URL}/chat/messages/stream`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ session_id: sid, message: msg }),
      signal: controller.signal,
    });

    if (!res.ok || !res.body) {
      const text = await res.text();
      throw new Error(text || `HTTP ${res.status}`);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let doneEvent = false;

    const handleEvent = (event: string, data: string) => {
      if (!data) return;
      let payload: any = null;
      try {
        payload = JSON.parse(data);
      } catch {
        return;
      }

      if (event === "chunk" || payload.type === "chunk") {
        const delta = typeof payload.delta === "string" ? payload.delta : "";
        if (delta) setStreamingText((prev) => prev + delta);
      } else if (event === "error" || payload.type === "error") {
        const msgText = payload.message || "Stream error";
        throw new Error(msgText);
      } else if (event === "done" || payload.type === "done") {
        doneEvent = true;
      }
    };

    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      const parts = buffer.split("\n\n");
      buffer = parts.pop() ?? "";
      for (const part of parts) {
        const lines = part.split("\n");
        let event = "";
        let data = "";
        for (const line of lines) {
          if (line.startsWith("event:")) event = line.slice(6).trim();
          if (line.startsWith("data:")) data += line.slice(5).trim();
        }
        handleEvent(event, data);
      }
    }

    if (doneEvent) {
      await refreshMessages(sid);
    }
  }

  async function onSend(e: React.FormEvent) {
    e.preventDefault();
    if (!sessionId) {
      setError("Session not ready yet. Try again in a moment.");
      return;
    }
    const msg = input.trim();
    if (!msg) return;

    setSending(true);
    setError(null);
    setInput("");
    setJob(null);
    setStreamingText("");

    try {
      if (responseMode === "stream") {
        setStreaming(true);
        await sendStreamMessage(sessionId, msg);
      } else {
        await sendFullMessage(sessionId, msg);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to send");
    } finally {
      setStreaming(false);
      setSending(false);
    }
  }

  return (
    <div className="min-h-screen p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Chat</h1>
          <p className="text-sm text-white/70">
            {me ? `Signed in as ${me.email}` : "Loading user..."}{" "}
            {sessionId ? `· session=${sessionId}` : "· creating session..."}
          </p>
        </div>

        <button
          className="border rounded px-3 py-1"
          onClick={() => {
            clearToken();
            router.push("/login");
          }}
        >
          Logout
        </button>
      </div>

      {error ? <p className="mt-4 text-sm text-red-600">{error}</p> : null}

      <div className="mt-6 grid gap-4">
        <div className="border rounded p-4 bg-white text-slate-900">
          <div className="flex items-center justify-between">
            <div className="text-sm font-medium">Messages</div>
            <div className="flex items-center gap-2">
              <button
                className="text-sm border rounded px-2 py-1 disabled:opacity-50"
                onClick={() => (sessionId ? refreshMessages(sessionId) : null)}
                disabled={!sessionId || refreshing}
                type="button"
              >
                {refreshing ? "Refreshing..." : "Refresh"}
              </button>
            </div>
          </div>

          {!sessionId ? (
            <p className="mt-4 text-sm text-gray-600">
              Select a session on the left, or click{" "}
              <span className="font-medium">New chat</span>.
            </p>
          ) : null}

          <div className="mt-3 flex items-center justify-between">
            <button
              className="text-sm border rounded px-2 py-1 disabled:opacity-50"
              onClick={loadOlderMessages}
              disabled={!sessionId || !nextBeforeId || loadingOlder}
              type="button"
              title={!nextBeforeId ? "No more history" : "Load older messages"}
            >
              {loadingOlder
                ? "Loading..."
                : nextBeforeId
                  ? "Load older"
                  : "No more history"}
            </button>

            {nextBeforeId ? (
              <span className="text-xs text-gray-500">
                next_before_id={nextBeforeId}
              </span>
            ) : (
              <span className="text-xs text-gray-500">end of history</span>
            )}
          </div>

          <div className="mt-4 space-y-3">
            {messages.length === 0 ? (
              <p className="text-sm text-gray-600">No messages yet.</p>
            ) : (
              messages.map((m) => (
                <div key={m.id} className="text-sm">
                  <div className="text-xs text-gray-500">
                    #{m.id} · {m.role} ·{" "}
                    {new Date(m.created_at).toLocaleString()}
                  </div>
                  <div className="mt-1 whitespace-pre-wrap">{m.content}</div>
                </div>
              ))
            )}
            {streaming ? (
              <div className="text-sm">
                <div className="text-xs text-gray-500">assistant · streaming</div>
                <div className="mt-1 whitespace-pre-wrap">
                  {streamingText || "…"}
                </div>
              </div>
            ) : null}
            <div ref={bottomRef} />
          </div>
        </div>

        <div className="border rounded p-4 bg-white text-slate-900">
          <div className="text-sm font-medium">Latest Job</div>
          <div className="mt-2">
            {job ? (
              <pre className="text-xs bg-gray-50 border rounded p-3 overflow-auto">
                {JSON.stringify(job, null, 2)}
              </pre>
            ) : (
              <p className="text-sm text-gray-600">No job yet.</p>
            )}
          </div>
        </div>

        <form onSubmit={onSend} className="border rounded p-4 bg-white text-slate-900">
          <label className="block text-sm font-medium">Send</label>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
            <span className="text-xs font-medium text-gray-600">Response mode</span>
            <div className="inline-flex rounded border p-0.5">
              <button
                type="button"
                className={`rounded px-2 py-1 ${
                  responseMode === "full" ? "bg-black text-white" : "text-gray-700"
                }`}
                onClick={() => setResponseMode("full")}
              >
                Full
              </button>
              <button
                type="button"
                className={`rounded px-2 py-1 ${
                  responseMode === "stream" ? "bg-black text-white" : "text-gray-700"
                }`}
                onClick={() => setResponseMode("stream")}
              >
                Stream
              </button>
            </div>
          </div>
          <div className="mt-2 flex gap-2">
            <input
              className="flex-1 border rounded px-3 py-2"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Type a message..."
              disabled={sending}
            />
            <button
              className="rounded bg-black text-white px-4 py-2 disabled:opacity-50"
              type="submit"
              disabled={sending || !sessionId}
            >
              {sending ? (responseMode === "stream" ? "Streaming..." : "Sending...") : "Send"}
            </button>
          </div>
          <p className="mt-2 text-xs text-gray-500">
            Pagination uses <code>before_id</code> and{" "}
            <code>next_before_id</code>. Session is persisted via{" "}
            <code>?session_id=...</code> so refresh will not lose it.
          </p>
        </form>
      </div>
    </div>
  );
}
