"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePollingTask } from "../../hooks/usePollingTask";
import type { Bookmark, BookmarkFormData, PersistentSession, RenameTarget, SessionsPanelProps } from "./terminalSessionTypes";

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

const jsonInit = (method: string, body: unknown = {}): RequestInit => ({
  method,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

function matches(query: string, ...fields: Array<string | undefined>): boolean {
  return query === "" || fields.some((field) => (field ?? "").toLowerCase().includes(query));
}

/**
 * The hub's terminal sessions that outlive a tab (running, ended) and the saved connections, polled every 10 s while
 * isOpen. Rows call the actions below; each refreshes the lists once the hub answers.
 */
export function useTerminalSessionLibrary({
  isOpen, onSessionSelect, onBookmarkConnect, onArchivedView,
}: Omit<SessionsPanelProps, "onClose">) {
  const [sessions, setSessions] = useState<PersistentSession[]>([]);
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [bookmarkDialogOpen, setBookmarkDialogOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState<RenameTarget | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const renameInputRef = useRef<HTMLInputElement | null>(null);
  const renameSubmittedRef = useRef(false);

  const fetchDataTask = useCallback(async (signal: AbortSignal) => {
    try {
      const [sessionsRes, bookmarksRes] = await Promise.all([
        fetch("/api/terminal/persistent-sessions", { signal }),
        fetch("/api/terminal/bookmarks", { signal }),
      ]);
      if (sessionsRes.ok) {
        const data = (await safeJSON(sessionsRes)) as { persistent_sessions?: PersistentSession[] } | null;
        if (!signal.aborted) setSessions(Array.isArray(data?.persistent_sessions) ? data.persistent_sessions : []);
      }
      if (bookmarksRes.ok) {
        const data = (await safeJSON(bookmarksRes)) as { bookmarks?: Bookmark[] } | null;
        if (!signal.aborted) setBookmarks(Array.isArray(data?.bookmarks) ? data.bookmarks : []);
      }
    } catch {
      // The lists keep their last answer until the next poll.
    }
  }, []);

  const fetchData = usePollingTask(fetchDataTask, 10_000, isOpen);

  useEffect(() => {
    if (!renameTarget) return undefined;
    renameSubmittedRef.current = false;
    const timer = setTimeout(() => renameInputRef.current?.focus(), 0);
    return () => clearTimeout(timer);
  }, [renameTarget]);

  const query = searchQuery.toLowerCase().trim();
  const byStatus = useCallback(
    (status: PersistentSession["status"]) =>
      sessions.filter((s) => s.status === status && matches(query, s.title, s.target)),
    [sessions, query],
  );
  const activeSessions = useMemo(() => byStatus("attached"), [byStatus]);
  const detachedSessions = useMemo(() => byStatus("detached"), [byStatus]);
  const archivedSessions = useMemo(() => byStatus("archived"), [byStatus]);

  // A saved connection with a running session shows as that session.
  const savedBookmarks = useMemo(() => {
    const linked = new Set(
      sessions.filter((s) => (s.status === "attached" || s.status === "detached") && s.bookmark_id).map((s) => s.bookmark_id),
    );
    return bookmarks.filter((b) => !linked.has(b.id) && matches(query, b.title, b.host, b.username));
  }, [bookmarks, sessions, query]);

  /** Back to a running session (attached elsewhere: open it here too). */
  const reattach = useCallback(
    async (session: PersistentSession, newTab = false) => {
      if (session.status === "archived") {
        onArchivedView(session.id, session.title || session.target);
        return;
      }
      if (session.status === "attached") {
        onSessionSelect("", session.id, "", newTab, session.target);
        return;
      }
      try {
        const res = await fetch(`/api/terminal/persistent-sessions/${encodeURIComponent(session.id)}/attach`, jsonInit("POST"));
        if (!res.ok) return;
        const data = (await safeJSON(res)) as { session?: { id?: string; persistent_session_id?: string } } | null;
        onSessionSelect(data?.session?.id ?? "", data?.session?.persistent_session_id ?? session.id, "", newTab, session.target);
        fetchData();
      } catch {
        // The row stays as it was; the next poll shows the hub's state.
      }
    },
    [onSessionSelect, onArchivedView, fetchData],
  );

  const connectBookmark = useCallback(
    async (bookmark: Bookmark, newTab = false) => {
      try {
        const res = await fetch(`/api/terminal/bookmarks/${encodeURIComponent(bookmark.id)}/connect`, jsonInit("POST"));
        if (!res.ok) return;
        onBookmarkConnect(bookmark.id, newTab, bookmark);
        fetchData();
      } catch {
        // Nothing opened; the row stays.
      }
    },
    [onBookmarkConnect, fetchData],
  );

  const saveAsBookmark = useCallback(
    async (session: PersistentSession) => {
      try {
        await fetch("/api/terminal/bookmarks", jsonInit("POST", { title: session.title || session.target, host: session.target }));
        fetchData();
      } catch {
        // The next poll shows whether it was saved.
      }
    },
    [fetchData],
  );

  const endSession = useCallback(
    async (sessionId: string) => {
      try {
        await fetch(`/api/terminal/persistent-sessions/${encodeURIComponent(sessionId)}`, { method: "DELETE" });
        fetchData();
      } catch {
        // The next poll shows whether it ended.
      }
    },
    [fetchData],
  );

  const deleteBookmark = useCallback(
    async (bookmarkId: string) => {
      try {
        await fetch(`/api/terminal/bookmarks/${encodeURIComponent(bookmarkId)}`, { method: "DELETE" });
        fetchData();
      } catch {
        // The next poll shows whether it was deleted.
      }
    },
    [fetchData],
  );

  const startRename = useCallback((target: RenameTarget) => {
    setRenameTarget(target);
    setRenameValue(target.currentTitle);
  }, []);

  const handleRenameSubmit = useCallback(async () => {
    if (renameSubmittedRef.current || !renameTarget) return;
    renameSubmittedRef.current = true;
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === renameTarget.currentTitle) {
      setRenameTarget(null);
      return;
    }
    const path = renameTarget.type === "session" ? "persistent-sessions" : "bookmarks";
    try {
      await fetch(`/api/terminal/${path}/${encodeURIComponent(renameTarget.id)}`, jsonInit("PUT", { title: trimmed }));
      fetchData();
    } catch {
      // The next poll shows the hub's title.
    }
    setRenameTarget(null);
  }, [renameTarget, renameValue, fetchData]);

  const handleBookmarkSave = useCallback(
    async (formData: BookmarkFormData) => {
      try {
        await fetch("/api/terminal/bookmarks", jsonInit("POST", formData));
        fetchData();
      } catch {
        // The next poll shows whether it was saved.
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
    renameTarget,
    setRenameTarget,
    renameValue,
    setRenameValue,
    renameInputRef,
    query,
    activeSessions,
    detachedSessions,
    archivedSessions,
    savedBookmarks,
    reattach,
    connectBookmark,
    saveAsBookmark,
    endSession,
    deleteBookmark,
    startRename,
    handleRenameSubmit,
    handleBookmarkSave,
  };
}

export type TerminalSessionLibrary = ReturnType<typeof useTerminalSessionLibrary>;
