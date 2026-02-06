"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { clearToken, getToken } from "@/lib/auth";
import FlowBackground from "@/components/FlowBackground";

type Session = {
  session_id: string;
  title?: string;
  provider: string;
  model: string;
  created_at: string;
  updated_at: string;
};

type ListSessionsResp = {
  sessions: Session[];
  next_before_id: number | null;
};

type Profile = {
  id: number;
  email: string;
  username: string;
};

const PROVIDER_DEFAULTS: Record<string, string> = {
  openrouter: "openrouter/auto",
  ollama: "llama3:latest",
};

const MODEL_PRESETS: Record<string, string[]> = {
  openrouter: [
    "openrouter/auto",
    "arcee-ai/trinity-large-preview:free",
    "stepfun/step-3.5-flash:free",
    "liquid/lfm-2.5-1.2b-thinking:free",
    "liquid/lfm-2.5-1.2b-instruct:free",
    "nvidia/nemotron-3-nano-30b-a3b:free",
    "arcee-ai/trinity-mini:free",
    "tngtech/tng-r1t-chimera:free",
  ],
  ollama: ["llama3:latest"],
};

export default function ChatLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const activeSessionId = searchParams.get("session_id") ?? "";

  const [sessions, setSessions] = useState<Session[]>([]);
  const [nextBeforeId, setNextBeforeId] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [menuOpenId, setMenuOpenId] = useState<string | null>(null);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [actionBusy, setActionBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<{ id: string; title: string } | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);
  const [profileLoading, setProfileLoading] = useState(false);
  const [profileError, setProfileError] = useState<string | null>(null);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [profileBusy, setProfileBusy] = useState(false);
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordMessage, setPasswordMessage] = useState<string | null>(null);
  const [deleteAccountConfirm, setDeleteAccountConfirm] = useState(false);
  const [deleteAccountPassword, setDeleteAccountPassword] = useState("");
  const [newProvider, setNewProvider] = useState("openrouter");
  const [newModel, setNewModel] = useState(PROVIDER_DEFAULTS.openrouter);

  const loadSessions = useCallback(async () => {
    setLoading(true);
    try {
      const data = await apiFetch<ListSessionsResp>("/chat/sessions?limit=30", { auth: true });
      setSessions(data.sessions ?? []);
      setNextBeforeId(data.next_before_id ?? null);
    } finally {
      setLoading(false);
    }
  }, []);

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
    const provider = newProvider.trim();
    const model = newModel.trim();
    const d = await apiFetch<{ session_id: string }>("/chat/sessions", {
      method: "POST",
      auth: true,
      body: JSON.stringify({
        provider,
        model,
      }),
    });

    // Go to new chat
    router.push(`/chat?session_id=${encodeURIComponent(d.session_id)}`);

    // Refresh sidebar (best-effort)
    loadSessions().catch(() => {});
  }

  async function loadProfile() {
    setProfileLoading(true);
    setProfileError(null);
    try {
      const data = await apiFetch<Profile>("/me", { auth: true });
      setProfile(data);
    } catch (e) {
      setProfileError(e instanceof Error ? e.message : "Failed to load profile");
    } finally {
      setProfileLoading(false);
    }
  }

  async function openProfile() {
    setProfileOpen(true);
    setPasswordMessage(null);
    setProfileError(null);
    if (!profile) {
      await loadProfile();
    }
  }

  function closeProfile() {
    setProfileOpen(false);
    setDeleteAccountConfirm(false);
    setDeleteAccountPassword("");
  }

  async function updatePassword() {
    setPasswordMessage(null);
    if (!oldPassword.trim() || !newPassword.trim()) {
      setPasswordMessage("Old and new password are required.");
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordMessage("New passwords do not match.");
      return;
    }
    setProfileBusy(true);
    try {
      await apiFetch("/me/password", {
        method: "PATCH",
        auth: true,
        body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
      });
      setOldPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setPasswordMessage("Password updated.");
    } catch (e) {
      setPasswordMessage(e instanceof Error ? e.message : "Failed to update password");
    } finally {
      setProfileBusy(false);
    }
  }

  async function deleteAccount() {
    if (!deleteAccountPassword.trim()) {
      setProfileError("Password required to delete account.");
      return;
    }
    setProfileBusy(true);
    try {
      await apiFetch("/me", {
        method: "DELETE",
        auth: true,
        body: JSON.stringify({ password: deleteAccountPassword }),
      });
      clearToken();
      router.replace("/login");
    } catch (e) {
      setProfileError(e instanceof Error ? e.message : "Failed to delete account");
    } finally {
      setProfileBusy(false);
    }
  }

  async function renameSession(sessionId: string) {
    const title = renameValue.trim();
    if (!title) return;
    setActionBusy(true);
    try {
      await apiFetch(`/chat/sessions/${encodeURIComponent(sessionId)}`, {
        method: "PATCH",
        auth: true,
        body: JSON.stringify({ title }),
      });
      setRenamingId(null);
      setRenameValue("");
      await loadSessions();
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "Failed to rename session");
    } finally {
      setActionBusy(false);
    }
  }

  async function deleteSession(sessionId: string) {
    setActionBusy(true);
    try {
      await apiFetch(`/chat/sessions/${encodeURIComponent(sessionId)}`, {
        method: "DELETE",
        auth: true,
      });
      if (sessionId === activeSessionId) {
        router.push("/chat");
      }
      await loadSessions();
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "Failed to delete session");
    } finally {
      setConfirmDelete(null);
      setActionBusy(false);
    }
  }

  useEffect(() => {
    if (!getToken()) {
      router.replace("/login");
      return;
    }
    loadSessions().catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname, loadSessions]);

  useEffect(() => {
    const handler = () => {
      loadSessions().catch(() => {});
    };
    window.addEventListener("chat:sessions:refresh", handler);
    return () => window.removeEventListener("chat:sessions:refresh", handler);
  }, [loadSessions]);

  useEffect(() => {
    setMenuOpenId(null);
    setRenamingId(null);
    setProfileOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const storedProvider = window.localStorage.getItem("chat:newProvider");
    const storedModel = window.localStorage.getItem("chat:newModel");
    if (storedProvider) {
      setNewProvider(storedProvider);
      setNewModel(storedModel || PROVIDER_DEFAULTS[storedProvider] || "");
      return;
    }
    if (storedModel) {
      setNewModel(storedModel);
    }
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem("chat:newProvider", newProvider);
    window.localStorage.setItem("chat:newModel", newModel);
  }, [newProvider, newModel]);

  return (
    <FlowBackground>
      <div className="min-h-screen flex">
        {/* Sidebar */}
        <aside className="w-72 border-r border-white/10 bg-white/5 p-3 text-white backdrop-blur flex flex-col gap-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]">
          <div className="rounded border border-white/10 bg-white/5 p-3">
            <div className="text-xs font-medium text-white/70">New session settings</div>
            <label className="mt-2 block text-[11px] text-white/60">Provider</label>
            <select
              className="mt-1 w-full rounded border border-white/15 bg-white/5 px-2 py-1 text-xs text-white"
              value={newProvider}
              onChange={(e) => {
                const next = e.target.value;
                const prevDefault = PROVIDER_DEFAULTS[newProvider] || "";
                setNewProvider(next);
                setNewModel((prev) => {
                  if (!prev || prev === prevDefault) {
                    return PROVIDER_DEFAULTS[next] || "";
                  }
                  return prev;
                });
              }}
            >
              <option value="openrouter">openrouter</option>
              <option value="ollama">ollama</option>
            </select>

            <label className="mt-3 block text-[11px] text-white/60">Model</label>
            <select
              className="mt-1 w-full rounded border border-white/15 bg-white/5 px-2 py-1 text-xs text-white"
              value={
                (MODEL_PRESETS[newProvider] || []).includes(newModel)
                  ? newModel
                  : "__custom__"
              }
              onChange={(e) => {
                const next = e.target.value;
                if (next !== "__custom__") {
                  setNewModel(next);
                }
              }}
            >
              {(MODEL_PRESETS[newProvider] || []).map((m) => (
                <option value={m} key={m}>
                  {m}
                </option>
              ))}
              <option value="__custom__">Custom…</option>
            </select>
            <input
              className="mt-2 w-full rounded border border-white/15 bg-white/5 px-2 py-1 text-xs text-white placeholder:text-white/40"
              value={newModel}
              onChange={(e) => setNewModel(e.target.value)}
              placeholder={PROVIDER_DEFAULTS[newProvider] || "model-name"}
            />
            <div className="mt-2 text-[11px] text-white/50">
              Used when you click “New chat”.
            </div>
          </div>

          <button className="border border-white/15 rounded px-3 py-2 text-sm text-white/90 hover:bg-white/10" onClick={createNewChat}>
            New chat
          </button>

          <div className="flex-1 overflow-auto">
            {loading && sessions.length === 0 ? (
              <div className="text-sm text-white/60">Loading…</div>
            ) : sessions.length === 0 ? (
              <div className="text-sm text-white/60">No sessions yet.</div>
            ) : (
              <div className="space-y-2">
                {sessions.map((s) => {
                  const active = s.session_id === activeSessionId;
                  const isRenaming = renamingId === s.session_id;
                  const itemClass = `block rounded border border-white/10 px-3 py-2 text-sm text-white/90 hover:bg-white/10 ${
                    active ? "bg-white/15 border-white/30" : ""
                  }`;

                  return (
                    <div
                      key={s.session_id}
                      className="relative group"
                      onMouseLeave={() => setMenuOpenId((id) => (id === s.session_id ? null : id))}
                    >
                      {isRenaming ? (
                        <div className={itemClass}>
                          <input
                            className="w-full rounded border border-white/15 bg-white/5 px-2 py-1 text-sm text-white placeholder:text-white/40"
                            value={renameValue}
                            onChange={(e) => setRenameValue(e.target.value)}
                            placeholder="New title"
                            maxLength={128}
                            autoFocus
                            onKeyDown={(e) => {
                              if (e.key === "Enter") {
                                e.preventDefault();
                                renameSession(s.session_id);
                              } else if (e.key === "Escape") {
                                setRenamingId(null);
                                setRenameValue("");
                              }
                            }}
                          />
                          <div className="mt-2 flex items-center gap-2">
                            <button
                              className="rounded border border-white/20 px-2 py-1 text-xs text-white/80 hover:bg-white/10 disabled:opacity-50"
                              onClick={() => renameSession(s.session_id)}
                              disabled={actionBusy}
                              type="button"
                            >
                              Save
                            </button>
                            <button
                              className="rounded border border-white/10 px-2 py-1 text-xs text-white/60 hover:bg-white/10"
                              onClick={() => {
                                setRenamingId(null);
                                setRenameValue("");
                              }}
                              type="button"
                            >
                              Cancel
                            </button>
                          </div>
                        </div>
                      ) : (
                        <Link
                          href={`/chat?session_id=${encodeURIComponent(s.session_id)}`}
                          className={itemClass}
                        >
                          <div className="font-medium truncate">{s.title || s.session_id}</div>
                          <div className="text-xs text-white/60 truncate">
                            {s.provider} · {s.model}
                          </div>
                        </Link>
                      )}

                      <div className="absolute right-2 top-2 flex items-center gap-1 opacity-0 transition group-hover:opacity-100">
                        <button
                          type="button"
                          className="rounded border border-white/15 bg-white/5 px-1.5 py-1 text-xs text-white/70 hover:bg-white/15"
                          onClick={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            setMenuOpenId((id) => (id === s.session_id ? null : s.session_id));
                          }}
                        >
                          ···
                        </button>
                      </div>

                      {menuOpenId === s.session_id ? (
                        <div className="absolute right-2 top-8 z-10 w-32 rounded border border-white/15 bg-slate-950/90 p-1 text-xs shadow-lg">
                          <button
                            type="button"
                            className="block w-full rounded px-2 py-1 text-left text-white/80 hover:bg-white/10"
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              setRenamingId(s.session_id);
                              setRenameValue(s.title ?? "");
                              setMenuOpenId(null);
                            }}
                          >
                            Rename
                          </button>
                          <button
                            type="button"
                            className="block w-full rounded px-2 py-1 text-left text-red-400 hover:bg-red-500/10"
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              setMenuOpenId(null);
                              setConfirmDelete({
                                id: s.session_id,
                                title: s.title || s.session_id,
                              });
                            }}
                          >
                            Delete
                          </button>
                        </div>
                      ) : null}
                    </div>
                  );
                })}

                <button
                  className="w-full border border-white/15 rounded px-3 py-2 text-sm text-white/80 disabled:opacity-50 hover:bg-white/10"
                  disabled={!nextBeforeId}
                  onClick={loadMore}
                >
                  {nextBeforeId ? "Load more" : "No more"}
                </button>
              </div>
            )}
          </div>

          <button
            className="border border-white/15 rounded px-3 py-2 text-sm text-white/80 hover:bg-white/10"
            onClick={openProfile}
            type="button"
          >
            Profile
          </button>

          <div className="text-xs text-white/40">
            Tip: sessions are the “left list”, messages are inside a session.
          </div>
        </aside>

        {/* Main */}
        <main className="flex-1">{children}</main>
      </div>

      {profileOpen ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 p-4"
          onClick={closeProfile}
        >
          <div
            className="w-full max-w-md rounded-lg border border-white/15 bg-slate-950/90 p-4 text-white shadow-xl backdrop-blur"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between">
              <div className="text-sm font-semibold">Profile</div>
              <button
                className="rounded px-2 py-1 text-xs text-white/70 hover:text-white"
                onClick={closeProfile}
                type="button"
                aria-label="Close"
              >
                ×
              </button>
            </div>

            {profileLoading ? (
              <div className="mt-3 text-xs text-white/70">Loading profile…</div>
            ) : profileError ? (
              <div className="mt-3 text-xs text-red-200">{profileError}</div>
            ) : profile ? (
              <div className="mt-3 space-y-2 text-xs text-white/80">
                <div>
                  <span className="text-white/50">Email:</span> {profile.email}
                </div>
                <div>
                  <span className="text-white/50">Username:</span> {profile.username}
                </div>
              </div>
            ) : null}

            <div className="mt-4 border-t border-white/10 pt-4">
              <div className="text-xs font-medium text-white/80">Change password</div>
              <div className="mt-2 space-y-2">
                <input
                  className="w-full rounded border border-white/15 bg-white/5 px-3 py-2 text-xs text-white placeholder:text-white/40"
                  type="password"
                  placeholder="Current password"
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                />
                <input
                  className="w-full rounded border border-white/15 bg-white/5 px-3 py-2 text-xs text-white placeholder:text-white/40"
                  type="password"
                  placeholder="New password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                />
                <input
                  className="w-full rounded border border-white/15 bg-white/5 px-3 py-2 text-xs text-white placeholder:text-white/40"
                  type="password"
                  placeholder="Confirm new password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                />
                {passwordMessage ? (
                  <div className="text-[11px] text-white/70">{passwordMessage}</div>
                ) : null}
                <button
                  className="rounded border border-white/20 px-3 py-1 text-xs text-white/80 hover:bg-white/10 disabled:opacity-50"
                  type="button"
                  onClick={updatePassword}
                  disabled={profileBusy}
                >
                  Update password
                </button>
              </div>
            </div>

            <div className="mt-4 border-t border-white/10 pt-4">
              <div className="text-xs font-medium text-white/80">Delete account</div>
              <p className="mt-1 text-[11px] text-white/50">
                This will permanently remove your account and all sessions.
              </p>
              {!deleteAccountConfirm ? (
                <button
                  className="mt-3 rounded bg-red-500/90 px-3 py-1 text-xs text-white hover:bg-red-500"
                  type="button"
                  onClick={() => {
                    setDeleteAccountConfirm(true);
                    setProfileError(null);
                  }}
                >
                  Delete account
                </button>
              ) : (
                <div className="mt-3 space-y-2">
                  <input
                    className="w-full rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-white placeholder:text-white/50"
                    type="password"
                    placeholder="Confirm password"
                    value={deleteAccountPassword}
                    onChange={(e) => setDeleteAccountPassword(e.target.value)}
                  />
                  <div className="flex items-center gap-2">
                    <button
                      className="rounded border border-white/20 px-3 py-1 text-xs text-white/80 hover:bg-white/10"
                      type="button"
                      onClick={() => setDeleteAccountConfirm(false)}
                    >
                      Cancel
                    </button>
                    <button
                      className="rounded bg-red-500/90 px-3 py-1 text-xs text-white hover:bg-red-500 disabled:opacity-50"
                      type="button"
                      onClick={deleteAccount}
                      disabled={profileBusy}
                    >
                      Confirm delete
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}

      {confirmDelete ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 p-4"
          onClick={() => setConfirmDelete(null)}
        >
          <div
            className="w-full max-w-sm rounded-lg border border-white/15 bg-slate-950/90 p-4 text-white shadow-xl backdrop-blur"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="text-sm font-semibold">Delete session?</div>
            <p className="mt-2 text-xs text-white/70">
              This will permanently delete{" "}
              <span className="text-white">{confirmDelete.title}</span> and all its messages.
            </p>
            <div className="mt-4 flex items-center justify-end gap-2">
              <button
                className="rounded border border-white/15 px-3 py-1 text-xs text-white/80 hover:bg-white/10"
                type="button"
                onClick={() => setConfirmDelete(null)}
                disabled={actionBusy}
              >
                Cancel
              </button>
              <button
                className="rounded bg-red-500/90 px-3 py-1 text-xs text-white hover:bg-red-500 disabled:opacity-50"
                type="button"
                onClick={() => deleteSession(confirmDelete.id)}
                disabled={actionBusy}
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </FlowBackground>
  );
}
