"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import SessionCard from "./SessionCard";
import BookmarkDialog from "./BookmarkDialog";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type PersistentSession = {
  id: string;
  actor_id: string;
  target: string;
  title: string;
  status: "attached" | "detached" | "archived";
  tmux_session_name: string;
  created_at: string;
  updated_at: string;
  last_attached_at?: string;
  last_detached_at?: string;
  bookmark_id?: string;
  archived_at?: string;
  pinned?: boolean;
};

type Bookmark = {
  id: string;
  actor_id: string;
  title: string;
  asset_id?: string;
  host?: string;
  port?: number;
  username?: string;
  tags?: string[];
  last_used_at?: string;
};

type BookmarkFormData = {
  title: string;
  asset_id?: string;
  host?: string;
  port?: number;
  username?: string;
  credential_profile_id?: string;
};

export type SessionsPanelProps = {
  isOpen: boolean;
  onClose: () => void;
  onSessionSelect: (
    sessionId: string,
    persistentSessionId: string,
    streamTicket: string,
    newTab: boolean,
  ) => void;
  onBookmarkConnect: (bookmarkId: string, newTab: boolean) => void;
  onArchivedView: (persistentSessionId: string, title: string) => void;
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function timeAgo(isoString: string | undefined): string {
  if (!isoString) return "";
  const diff = Date.now() - new Date(isoString).getTime();
  if (!Number.isFinite(diff) || diff < 0) return "just now";
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function durationSince(isoString: string | undefined): string {
  if (!isoString) return "";
  const diff = Date.now() - new Date(isoString).getTime();
  if (!Number.isFinite(diff) || diff < 0) return "0s";
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Context Menu
// ---------------------------------------------------------------------------

type ContextMenuState = {
  x: number;
  y: number;
  type: "active" | "detached" | "archived" | "saved";
  sessionId?: string;
  bookmarkId?: string;
  hasBookmark?: boolean;
};

type ContextMenuOption = {
  label: string;
  action: () => void;
  danger?: boolean;
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function SessionsPanel({
  isOpen,
  onClose,
  onSessionSelect,
  onBookmarkConnect,
  onArchivedView,
}: SessionsPanelProps) {
  const [sessions, setSessions] = useState<PersistentSession[]>([]);
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [bookmarkDialogOpen, setBookmarkDialogOpen] = useState(false);
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [renameTarget, setRenameTarget] = useState<{
    type: "session" | "bookmark";
    id: string;
    currentTitle: string;
  } | null>(null);
  const [renameValue, setRenameValue] = useState("");

  const panelRef = useRef<HTMLDivElement | null>(null);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const renameInputRef = useRef<HTMLInputElement | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const renameSubmittedRef = useRef(false);

  // -----------------------------------------------------------------------
  // Data fetching
  // -----------------------------------------------------------------------

  const fetchData = useCallback(async () => {
    try {
      const [sessionsRes, bookmarksRes] = await Promise.all([
        fetch("/api/terminal/persistent-sessions"),
        fetch("/api/terminal/bookmarks"),
      ]);

      if (sessionsRes.ok) {
        const data = (await safeJSON(sessionsRes)) as {
          persistent_sessions?: PersistentSession[];
        } | null;
        setSessions(data?.persistent_sessions ?? []);
      }

      if (bookmarksRes.ok) {
        const data = (await safeJSON(bookmarksRes)) as {
          bookmarks?: Bookmark[];
        } | null;
        setBookmarks(data?.bookmarks ?? []);
      }
    } catch {
      // Silently ignore fetch failures — data stays stale.
    }
  }, []);

  useEffect(() => {
    if (!isOpen) {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
      return;
    }

    fetchData();
    pollRef.current = setInterval(fetchData, 10_000);

    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [isOpen, fetchData]);

  // Focus search input on open.
  useEffect(() => {
    if (isOpen) {
      const timer = setTimeout(() => searchInputRef.current?.focus(), 80);
      return () => clearTimeout(timer);
    }
  }, [isOpen]);

  // Close context menu on outside click.
  useEffect(() => {
    if (!contextMenu) return;
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setContextMenu(null);
      }
    };
    // Use capture to catch clicks before they propagate.
    document.addEventListener("mousedown", handler, true);
    return () => document.removeEventListener("mousedown", handler, true);
  }, [contextMenu]);

  // Close context menu on Escape.
  useEffect(() => {
    if (!contextMenu) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setContextMenu(null);
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [contextMenu]);

  // Focus rename input when rename starts.
  useEffect(() => {
    if (renameTarget) {
      renameSubmittedRef.current = false;
      const timer = setTimeout(() => renameInputRef.current?.focus(), 0);
      return () => clearTimeout(timer);
    }
  }, [renameTarget]);

  // -----------------------------------------------------------------------
  // Grouped & filtered data
  // -----------------------------------------------------------------------

  const query = searchQuery.toLowerCase().trim();

  const activeSessions = useMemo(
    () =>
      sessions.filter(
        (s) =>
          s.status === "attached" &&
          (query === "" ||
            s.title.toLowerCase().includes(query) ||
            s.target.toLowerCase().includes(query)),
      ),
    [sessions, query],
  );

  const detachedSessions = useMemo(
    () =>
      sessions.filter(
        (s) =>
          s.status === "detached" &&
          (query === "" ||
            s.title.toLowerCase().includes(query) ||
            s.target.toLowerCase().includes(query)),
      ),
    [sessions, query],
  );

  const archivedSessions = useMemo(
    () =>
      sessions.filter(
        (s) =>
          s.status === "archived" &&
          (query === "" ||
            s.title.toLowerCase().includes(query) ||
            s.target.toLowerCase().includes(query)),
      ),
    [sessions, query],
  );

  // Bookmarks that have no active/detached persistent session.
  const linkedBookmarkIds = useMemo(() => {
    const ids = new Set<string>();
    for (const s of sessions) {
      if ((s.status === "attached" || s.status === "detached") && s.bookmark_id) {
        ids.add(s.bookmark_id);
      }
    }
    return ids;
  }, [sessions]);

  const savedBookmarks = useMemo(
    () =>
      bookmarks.filter(
        (b) =>
          !linkedBookmarkIds.has(b.id) &&
          (query === "" ||
            b.title.toLowerCase().includes(query) ||
            (b.host ?? "").toLowerCase().includes(query) ||
            (b.username ?? "").toLowerCase().includes(query)),
      ),
    [bookmarks, linkedBookmarkIds, query],
  );

  // -----------------------------------------------------------------------
  // Interaction handlers
  // -----------------------------------------------------------------------

  const isNewTab = useCallback((e: React.MouseEvent) => {
    return e.shiftKey || e.button === 1;
  }, []);

  const handleSessionClick = useCallback(
    async (session: PersistentSession, e: React.MouseEvent) => {
      const newTab = isNewTab(e);

      if (session.status === "archived") {
        onArchivedView(session.id, session.title || session.target);
        return;
      }

      if (session.status === "attached") {
        // Already attached — just select it.
        onSessionSelect("", session.id, "", newTab);
        return;
      }

      // Detached — attach first.
      try {
        const res = await fetch(
          `/api/terminal/persistent-sessions/${encodeURIComponent(session.id)}/attach`,
          { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" },
        );
        if (res.ok) {
          const data = (await safeJSON(res)) as {
            session?: { id?: string; persistent_session_id?: string };
          } | null;
          onSessionSelect(
            data?.session?.id ?? "",
            data?.session?.persistent_session_id ?? session.id,
            "",
            newTab,
          );
          // Refresh data after attach.
          fetchData();
        }
      } catch {
        // Silently ignore.
      }
    },
    [isNewTab, onSessionSelect, onArchivedView, fetchData],
  );

  const handleBookmarkClick = useCallback(
    async (bookmark: Bookmark, e: React.MouseEvent) => {
      const newTab = isNewTab(e);

      try {
        const res = await fetch(
          `/api/terminal/bookmarks/${encodeURIComponent(bookmark.id)}/connect`,
          { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" },
        );
        if (res.ok) {
          onBookmarkConnect(bookmark.id, newTab);
          fetchData();
        }
      } catch {
        // Silently ignore.
      }
    },
    [isNewTab, onBookmarkConnect, fetchData],
  );

  // -----------------------------------------------------------------------
  // Save as Bookmark
  // -----------------------------------------------------------------------

  const handleSaveAsBookmark = useCallback(
    async (session: PersistentSession) => {
      try {
        await fetch("/api/terminal/bookmarks", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            title: session.title || session.target,
            host: session.target,
          }),
        });
        fetchData();
      } catch {
        // Silently ignore.
      }
    },
    [fetchData],
  );

  // -----------------------------------------------------------------------
  // Context menu actions
  // -----------------------------------------------------------------------

  const handleContextMenu = useCallback(
    (
      e: React.MouseEvent,
      type: ContextMenuState["type"],
      sessionId?: string,
      bookmarkId?: string,
      hasBookmark?: boolean,
    ) => {
      e.preventDefault();
      e.stopPropagation();
      const rect = panelRef.current?.getBoundingClientRect();
      if (!rect) return;
      const MENU_WIDTH = 180;
      const MENU_HEIGHT = 200; // approximate
      const x = Math.min(e.clientX - rect.left, rect.width - MENU_WIDTH);
      const y = Math.min(e.clientY - rect.top, rect.height - MENU_HEIGHT);
      setContextMenu({ x, y, type, sessionId, bookmarkId, hasBookmark });
    },
    [],
  );

  const contextMenuOptions = useMemo((): ContextMenuOption[] => {
    if (!contextMenu) return [];
    const opts: ContextMenuOption[] = [];

    // Rename
    if (contextMenu.type !== "archived") {
      opts.push({
        label: "Rename\u2026",
        action: () => {
          if (contextMenu.type === "saved" && contextMenu.bookmarkId) {
            const bm = bookmarks.find((b) => b.id === contextMenu.bookmarkId);
            setRenameTarget({
              type: "bookmark",
              id: contextMenu.bookmarkId,
              currentTitle: bm?.title ?? "",
            });
            setRenameValue(bm?.title ?? "");
          } else if (contextMenu.sessionId) {
            const s = sessions.find((sess) => sess.id === contextMenu.sessionId);
            setRenameTarget({
              type: "session",
              id: contextMenu.sessionId,
              currentTitle: s?.title ?? "",
            });
            setRenameValue(s?.title ?? "");
          }
          setContextMenu(null);
        },
      });
    }

    // Save as Bookmark (active/detached without a bookmark)
    if (
      (contextMenu.type === "active" || contextMenu.type === "detached") &&
      !contextMenu.hasBookmark
    ) {
      opts.push({
        label: "Save as Bookmark",
        action: () => {
          if (contextMenu.sessionId) {
            const s = sessions.find((sess) => sess.id === contextMenu.sessionId);
            if (s) {
              handleSaveAsBookmark(s);
            }
          }
          setContextMenu(null);
        },
      });
    }

    // Open in New Tab
    opts.push({
      label: "Open in New Tab",
      action: () => {
        if (contextMenu.type === "saved" && contextMenu.bookmarkId) {
          const bm = bookmarks.find((b) => b.id === contextMenu.bookmarkId);
          if (bm) {
            handleBookmarkClick(bm, { shiftKey: true, button: 0 } as unknown as React.MouseEvent);
          }
        } else if (contextMenu.sessionId) {
          const s = sessions.find((sess) => sess.id === contextMenu.sessionId);
          if (s) {
            handleSessionClick(s, { shiftKey: true, button: 0 } as unknown as React.MouseEvent);
          }
        }
        setContextMenu(null);
      },
    });

    // Terminate Session (active/detached)
    if (contextMenu.type === "active" || contextMenu.type === "detached") {
      opts.push({
        label: "Terminate Session",
        danger: true,
        action: async () => {
          if (contextMenu.sessionId) {
            try {
              await fetch(
                `/api/terminal/persistent-sessions/${encodeURIComponent(contextMenu.sessionId)}`,
                { method: "DELETE" },
              );
              fetchData();
            } catch {
              // Silently ignore.
            }
          }
          setContextMenu(null);
        },
      });
    }

    // Delete Bookmark (saved)
    if (contextMenu.type === "saved" && contextMenu.bookmarkId) {
      opts.push({
        label: "Delete Bookmark",
        danger: true,
        action: async () => {
          if (contextMenu.bookmarkId) {
            try {
              await fetch(
                `/api/terminal/bookmarks/${encodeURIComponent(contextMenu.bookmarkId)}`,
                { method: "DELETE" },
              );
              fetchData();
            } catch {
              // Silently ignore.
            }
          }
          setContextMenu(null);
        },
      });
    }

    return opts;
  }, [contextMenu, sessions, bookmarks, handleSessionClick, handleBookmarkClick, handleSaveAsBookmark, fetchData]);

  // -----------------------------------------------------------------------
  // Rename
  // -----------------------------------------------------------------------

  const handleRenameSubmit = useCallback(async () => {
    if (renameSubmittedRef.current) return;
    if (!renameTarget) return;
    renameSubmittedRef.current = true;
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === renameTarget.currentTitle) {
      setRenameTarget(null);
      return;
    }

    try {
      if (renameTarget.type === "session") {
        await fetch(
          `/api/terminal/persistent-sessions/${encodeURIComponent(renameTarget.id)}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ title: trimmed }),
          },
        );
      } else {
        await fetch(
          `/api/terminal/bookmarks/${encodeURIComponent(renameTarget.id)}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ title: trimmed }),
          },
        );
      }
      fetchData();
    } catch {
      // Silently ignore.
    }

    setRenameTarget(null);
  }, [renameTarget, renameValue, fetchData]);

  // -----------------------------------------------------------------------
  // Bookmark dialog handlers
  // -----------------------------------------------------------------------

  const handleBookmarkSave = useCallback(
    async (formData: BookmarkFormData) => {
      try {
        await fetch("/api/terminal/bookmarks", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(formData),
        });
        fetchData();
      } catch {
        // Silently ignore.
      }
      setBookmarkDialogOpen(false);
    },
    [fetchData],
  );

  // -----------------------------------------------------------------------
  // Render helpers
  // -----------------------------------------------------------------------

  if (!isOpen) return null;

  const renderSectionHeader = (
    label: string,
    count: number,
    color: string,
    icon: string,
  ) => (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 6,
        padding: "8px 10px 4px",
        userSelect: "none",
      }}
    >
      <span style={{ fontSize: 10, color }}>{icon}</span>
      <span
        style={{
          fontSize: 10,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.06em",
          color: "#999",
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontSize: 9,
          color: "#666",
          marginLeft: "auto",
        }}
      >
        {count}
      </span>
    </div>
  );

  const renderRenameInline = (id: string, type: "session" | "bookmark") => {
    if (!renameTarget || renameTarget.id !== id || renameTarget.type !== type) {
      return null;
    }
    return (
      <div style={{ padding: "4px 10px" }}>
        <input
          ref={renameInputRef}
          type="text"
          value={renameValue}
          onChange={(e) => setRenameValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              handleRenameSubmit();
            } else if (e.key === "Escape") {
              setRenameTarget(null);
            }
          }}
          onBlur={handleRenameSubmit}
          style={{
            width: "100%",
            backgroundColor: "#1a1a2e",
            border: "1px solid #444",
            borderRadius: 3,
            padding: "3px 6px",
            fontSize: 11,
            color: "#e0e0e0",
            outline: "none",
          }}
        />
      </div>
    );
  };

  return (
    <>
      <div
        ref={panelRef}
        style={{
          width: 260,
          height: "100%",
          backgroundColor: "#12122a",
          borderRight: "1px solid #2a2a3e",
          display: "flex",
          flexDirection: "column",
          flexShrink: 0,
          position: "relative",
          overflow: "hidden",
        }}
      >
        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "10px 10px 8px",
            borderBottom: "1px solid #2a2a3e",
          }}
        >
          <span
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: "#ccc",
              letterSpacing: "0.02em",
            }}
          >
            Sessions
          </span>
          <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <button
              type="button"
              onClick={() => setBookmarkDialogOpen(true)}
              style={{
                fontSize: 10,
                fontWeight: 600,
                color: "#60a5fa",
                backgroundColor: "rgba(96, 165, 250, 0.12)",
                border: "1px solid rgba(96, 165, 250, 0.25)",
                borderRadius: 4,
                padding: "2px 8px",
                cursor: "pointer",
                lineHeight: "16px",
              }}
            >
              + New
            </button>
            <button
              type="button"
              onClick={onClose}
              aria-label="Close sessions panel"
              style={{
                fontSize: 14,
                color: "#666",
                backgroundColor: "transparent",
                border: "none",
                borderRadius: 3,
                padding: "2px 4px",
                cursor: "pointer",
                lineHeight: "16px",
              }}
            >
              ×
            </button>
          </div>
        </div>

        {/* Search */}
        <div style={{ padding: "6px 10px" }}>
          <input
            ref={searchInputRef}
            type="text"
            placeholder="Filter sessions..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{
              width: "100%",
              backgroundColor: "#1a1a2e",
              border: "1px solid #2a2a3e",
              borderRadius: 4,
              padding: "5px 8px",
              fontSize: 11,
              color: "#ccc",
              outline: "none",
            }}
          />
        </div>

        {/* Scrollable session list */}
        <div
          style={{
            flex: 1,
            overflowY: "auto",
            overflowX: "hidden",
            paddingBottom: 12,
          }}
        >
          {/* Active Sessions */}
          {activeSessions.length > 0 && (
            <div>
              {renderSectionHeader("Active", activeSessions.length, "#4ade80", "●")}
              {activeSessions.map((s) => (
                <div key={s.id} style={{ padding: "0 6px" }}>
                  {renameTarget?.id === s.id ? (
                    renderRenameInline(s.id, "session")
                  ) : (
                    <SessionCard
                      type="active"
                      title={s.title || s.target}
                      subtitle={s.target}
                      metadata={durationSince(s.last_attached_at)}
                      onClick={(e) => handleSessionClick(s, e)}
                      onContextMenu={(e) =>
                        handleContextMenu(e, "active", s.id, s.bookmark_id, !!s.bookmark_id)
                      }
                    />
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Detached Sessions */}
          {detachedSessions.length > 0 && (
            <div>
              {renderSectionHeader("Detached", detachedSessions.length, "#facc15", "◐")}
              {detachedSessions.map((s) => (
                <div key={s.id} style={{ padding: "0 6px" }}>
                  {renameTarget?.id === s.id ? (
                    renderRenameInline(s.id, "session")
                  ) : (
                    <SessionCard
                      type="detached"
                      title={s.title || s.target}
                      subtitle={s.target}
                      metadata={timeAgo(s.last_detached_at)}
                      onClick={(e) => handleSessionClick(s, e)}
                      onContextMenu={(e) =>
                        handleContextMenu(e, "detached", s.id, s.bookmark_id, !!s.bookmark_id)
                      }
                    />
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Archived Sessions */}
          {archivedSessions.length > 0 && (
            <div>
              {renderSectionHeader("Archived", archivedSessions.length, "#666", "▫")}
              {archivedSessions.map((s) => (
                <div key={s.id} style={{ padding: "0 6px" }}>
                  <SessionCard
                    type="archived"
                    title={s.title || s.target}
                    subtitle={s.target}
                    metadata={timeAgo(s.archived_at)}
                    onClick={(e) => handleSessionClick(s, e)}
                    onContextMenu={(e) =>
                      handleContextMenu(e, "archived", s.id)
                    }
                  />
                </div>
              ))}
            </div>
          )}

          {/* Saved Bookmarks */}
          {savedBookmarks.length > 0 && (
            <div>
              {renderSectionHeader("Saved", savedBookmarks.length, "#60a5fa", "☆")}
              {savedBookmarks.map((b) => (
                <div key={b.id} style={{ padding: "0 6px" }}>
                  {renameTarget?.id === b.id ? (
                    renderRenameInline(b.id, "bookmark")
                  ) : (
                    <SessionCard
                      type="saved"
                      title={b.title}
                      subtitle={
                        b.host
                          ? `${b.username ? b.username + "@" : ""}${b.host}${b.port && b.port !== 22 ? ":" + b.port : ""}`
                          : "No host configured"
                      }
                      isAssetLinked={!!b.asset_id}
                      onClick={(e) => handleBookmarkClick(b, e)}
                      onContextMenu={(e) =>
                        handleContextMenu(e, "saved", undefined, b.id)
                      }
                    />
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Empty state */}
          {activeSessions.length === 0 &&
            detachedSessions.length === 0 &&
            archivedSessions.length === 0 &&
            savedBookmarks.length === 0 && (
              <div
                style={{
                  padding: "24px 16px",
                  textAlign: "center",
                  color: "#666",
                  fontSize: 11,
                }}
              >
                {query
                  ? "No sessions match your filter."
                  : "No sessions or bookmarks yet."}
              </div>
            )}
        </div>

        {/* Context Menu */}
        {contextMenu && contextMenuOptions.length > 0 && (
          <div
            style={{
              position: "absolute",
              top: contextMenu.y,
              left: contextMenu.x,
              backgroundColor: "#1e1e2e",
              border: "1px solid #3a3a4e",
              borderRadius: 6,
              boxShadow: "0 8px 24px rgba(0, 0, 0, 0.5)",
              padding: "4px 0",
              zIndex: 200,
              minWidth: 160,
            }}
          >
            {contextMenuOptions.map((opt) => (
              <button
                key={opt.label}
                type="button"
                onClick={opt.action}
                style={{
                  display: "block",
                  width: "100%",
                  textAlign: "left",
                  padding: "6px 12px",
                  fontSize: 11,
                  color: opt.danger ? "#f87171" : "#ccc",
                  backgroundColor: "transparent",
                  border: "none",
                  cursor: "pointer",
                  whiteSpace: "nowrap",
                }}
                onMouseEnter={(e) => {
                  (e.currentTarget as HTMLButtonElement).style.backgroundColor = "#2a2a3e";
                }}
                onMouseLeave={(e) => {
                  (e.currentTarget as HTMLButtonElement).style.backgroundColor = "transparent";
                }}
              >
                {opt.label}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Bookmark Dialog */}
      <BookmarkDialog
        isOpen={bookmarkDialogOpen}
        onClose={() => setBookmarkDialogOpen(false)}
        onSave={handleBookmarkSave}
      />
    </>
  );
}
