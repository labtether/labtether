"use client";

import SessionCard from "./SessionCard";
import BookmarkDialog from "./BookmarkDialog";
import { useTerminalSessionLibrary } from "./useTerminalSessionLibrary";
import type { SessionsPanelProps } from "./terminalSessionTypes";
export type { SessionsPanelProps } from "./terminalSessionTypes";

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

export default function SessionsPanel({
  isOpen,
  onClose,
  onSessionSelect,
  onBookmarkConnect,
  onArchivedView,
}: SessionsPanelProps) {
  const {
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
  } = useTerminalSessionLibrary({ isOpen, onSessionSelect, onBookmarkConnect, onArchivedView });

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
