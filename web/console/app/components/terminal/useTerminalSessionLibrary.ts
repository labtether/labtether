"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PersistentSession, Bookmark, BookmarkFormData, ContextMenuState, ContextMenuOption, SessionsPanelProps } from "./terminalSessionTypes";

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

export function useTerminalSessionLibrary({
  isOpen, onSessionSelect, onBookmarkConnect, onArchivedView,
}: Omit<SessionsPanelProps, "onClose">) {
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

  return {
    searchQuery,
    setSearchQuery,
    bookmarkDialogOpen,
    setBookmarkDialogOpen,
    contextMenu,
    renameTarget,
    setRenameTarget,
    renameValue,
    setRenameValue,
    panelRef,
    searchInputRef,
    renameInputRef,
    query,
    activeSessions,
    detachedSessions,
    archivedSessions,
    savedBookmarks,
    handleSessionClick,
    handleBookmarkClick,
    handleContextMenu,
    contextMenuOptions,
    handleRenameSubmit,
    handleBookmarkSave,
  };
}
