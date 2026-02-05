"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
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

export default function ChatLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const activeSessionId = searchParams.get("session_id") ?? "";

  const [sessions, setSessions] = useState<Session[]>([]);
  const [nextBeforeId, setNextBeforeId] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);

  async function loadSessions(replace = true) {
    setLoading(true);
    try {
      const data = await apiFetch<ListSessionsResp>("/chat/sessions?limit=30", { auth: true });
      setSessions(data.sessions ?? []);
      setNextBeforeId(data.next_before_id ?? null);
    } finally {
      setLoading(false);
    }
  }

  async function loadMore() {
    if (!nextBeforeId) return;
    const data = await apiFetch<ListSessionsResp>(
      `/chat/sessions?limit=30&before_id=${encodeURIComponent(String(nextBeforeId))}`,
      { auth: true }
    );
    const incoming = data.sessions ?? [];
    setSessions((prev) => {
      const map = new Map<string, Session>();
      for (const s of prev) map.set(s.session_id, s);
      for (const s of incoming) map.set(s.session_id, s);
      return Array.from(map.values()).sort(
        (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
      );
    });
    setNextBeforeId(data.next_before_id ?? null);
  }

  async function createNewChat() {
    const d = await apiFetch<{ session_id: string }>("/chat/sessions", {
      method: "POST",
      auth: true,
      body: JSON.stringify({}),
    });

    // Go to new chat
    router.push(`/chat?session_id=${encodeURIComponent(d.session_id)}`);

    // Refresh sidebar (best-effort)
    loadSessions().catch(() => {});
  }

  useEffect(() => {
    if (!getToken()) {
      router.replace("/login");
      return;
    }
    loadSessions().catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  return (
    <FlowBackground>
      <div className="min-h-screen flex">
        {/* Sidebar */}
        <aside className="w-72 border-r border-white/10 bg-white/90 p-3 text-slate-900 backdrop-blur flex flex-col gap-3">
          <button className="border rounded px-3 py-2 text-sm" onClick={createNewChat}>
            New chat
          </button>

          <div className="flex-1 overflow-auto">
            {loading && sessions.length === 0 ? (
              <div className="text-sm text-gray-600">Loading…</div>
            ) : sessions.length === 0 ? (
              <div className="text-sm text-gray-600">No sessions yet.</div>
            ) : (
              <div className="space-y-2">
                {sessions.map((s) => {
                  const active = s.session_id === activeSessionId;
                  return (
                    <Link
                      key={s.session_id}
                      href={`/chat?session_id=${encodeURIComponent(s.session_id)}`}
                      className={`block rounded border px-3 py-2 text-sm hover:bg-gray-50 ${
                        active ? "bg-gray-50 border-gray-400" : ""
                      }`}
                    >
                      <div className="font-medium truncate">{s.session_id}</div>
                      <div className="text-xs text-gray-600 truncate">
                        {s.provider} · {s.model}
                      </div>
                    </Link>
                  );
                })}

                <button
                  className="w-full border rounded px-3 py-2 text-sm disabled:opacity-50"
                  disabled={!nextBeforeId}
                  onClick={loadMore}
                >
                  {nextBeforeId ? "Load more" : "No more"}
                </button>
              </div>
            )}
          </div>

          <div className="text-xs text-gray-500">
            Tip: sessions are the “left list”, messages are inside a session.
          </div>
        </aside>

        {/* Main */}
        <main className="flex-1">{children}</main>
      </div>
    </FlowBackground>
  );
}
