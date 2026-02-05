"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { getToken } from "@/lib/auth";
import FlowBackground from "@/components/FlowBackground";

type Session = {
  session_id: string;
  provider: string;
  model: string;
  created_at: string;
  updated_at: string;
};

type ListSessionsResp = {
  sessions: Session[];
  next_before_id: number | null;
};

export default function SessionsPage() {
  const router = useRouter();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [nextBeforeId, setNextBeforeId] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function loadFirst() {
    setLoading(true);
    setError(null);
    try {
      const data = await apiFetch<ListSessionsResp>("/chat/sessions?limit=20", { auth: true });
      setSessions(data.sessions ?? []);
      setNextBeforeId(data.next_before_id ?? null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  }

  async function loadMore() {
    if (!nextBeforeId) return;
    setLoadingMore(true);
    setError(null);
    try {
      const data = await apiFetch<ListSessionsResp>(
        `/chat/sessions?limit=20&before_id=${encodeURIComponent(String(nextBeforeId))}`,
        { auth: true }
      );
      const incoming = data.sessions ?? [];
      setSessions((prev) => {
        const map = new Map<string, Session>();
        for (const s of prev) map.set(s.session_id, s);
        for (const s of incoming) map.set(s.session_id, s);
        // best-effort sort by updated_at desc (since id isn't exposed)
        return Array.from(map.values()).sort(
          (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
        );
      });
      setNextBeforeId(data.next_before_id ?? null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load more");
    } finally {
      setLoadingMore(false);
    }
  }

  useEffect(() => {
    if (!getToken()) {
      router.replace("/login");
      return;
    }
    loadFirst().catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <FlowBackground>
      <div className="min-h-screen p-6">
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">Sessions</h1>
          <Link className="border rounded px-3 py-1" href="/chat">
            New
          </Link>
        </div>

        {error ? <p className="mt-4 text-sm text-red-400">{error}</p> : null}

        <div className="mt-4">
          <button className="text-sm border rounded px-2 py-1" onClick={loadFirst} disabled={loading}>
            {loading ? "Loading..." : "Refresh"}
          </button>
        </div>

        <div className="mt-6 space-y-3">
          {sessions.length === 0 ? (
            <p className="text-sm text-white/70">{loading ? "Loading..." : "No sessions."}</p>
          ) : (
            sessions.map((s) => (
              <Link
                key={s.session_id}
                href={`/chat?session_id=${encodeURIComponent(s.session_id)}`}
                className="block rounded border border-white/15 bg-white/5 p-3 transition hover:bg-white/10"
              >
                <div className="text-sm font-medium">{s.session_id}</div>
                <div className="mt-1 text-xs text-white/70">
                  {s.provider} · {s.model} · updated {new Date(s.updated_at).toLocaleString()}
                </div>
              </Link>
            ))
          )}
        </div>

        <div className="mt-6">
          <button
            className="text-sm border rounded px-3 py-2 disabled:opacity-50"
            onClick={loadMore}
            disabled={!nextBeforeId || loadingMore}
          >
            {loadingMore ? "Loading..." : nextBeforeId ? "Load more" : "No more"}
          </button>
        </div>
      </div>
    </FlowBackground>
  );
}
