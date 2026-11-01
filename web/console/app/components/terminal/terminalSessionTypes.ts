export type PersistentSession = {
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

export type Bookmark = {
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

export type BookmarkFormData = {
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
  /** A running session was reattached (or is already attached); target is the device it runs on. */
  onSessionSelect: (
    sessionId: string,
    persistentSessionId: string,
    streamTicket: string,
    newTab: boolean,
    target?: string,
  ) => void;
  /** A saved connection was opened. */
  onBookmarkConnect: (bookmarkId: string, newTab: boolean, bookmark?: Bookmark) => void;
  onArchivedView: (persistentSessionId: string, title: string) => void;
};

/** What a rename is editing: a session's title or a saved connection's name. */
export type RenameTarget = {
  type: "session" | "bookmark";
  id: string;
  currentTitle: string;
};
