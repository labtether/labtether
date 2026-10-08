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
  onSessionSelect: (
    sessionId: string,
    persistentSessionId: string,
    streamTicket: string,
    newTab: boolean,
  ) => void;
  onBookmarkConnect: (bookmarkId: string, newTab: boolean) => void;
  onArchivedView: (persistentSessionId: string, title: string) => void;
};

export type ContextMenuState = {
  x: number;
  y: number;
  type: "active" | "detached" | "archived" | "saved";
  sessionId?: string;
  bookmarkId?: string;
  hasBookmark?: boolean;
};

export type ContextMenuOption = {
  label: string;
  action: () => void;
  danger?: boolean;
};
