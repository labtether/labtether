"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { X } from "lucide-react";

type Bookmark = {
  id: string;
  title: string;
  asset_id?: string;
  host?: string;
  port?: number;
  username?: string;
  credential_profile_id?: string;
};

type BookmarkFormData = {
  title: string;
  asset_id?: string;
  host?: string;
  port?: number;
  username?: string;
  credential_profile_id?: string;
};

type BookmarkDialogProps = {
  isOpen: boolean;
  onClose: () => void;
  onSave: (bookmark: BookmarkFormData) => void;
  editBookmark?: Bookmark;
};

export default function BookmarkDialog({
  isOpen,
  onClose,
  onSave,
  editBookmark,
}: BookmarkDialogProps) {
  const [title, setTitle] = useState("");
  const [assetId, setAssetId] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("22");
  const [username, setUsername] = useState("");
  const [credentialProfileId, setCredentialProfileId] = useState("");

  const titleInputRef = useRef<HTMLInputElement | null>(null);
  const prevOpenRef = useRef(false);

  // Pre-fill fields when the dialog transitions from closed to open.
  useEffect(() => {
    if (isOpen && !prevOpenRef.current) {
      if (editBookmark) {
        setTitle(editBookmark.title);
        setAssetId(editBookmark.asset_id ?? "");
        setHost(editBookmark.host ?? "");
        setPort(editBookmark.port != null ? String(editBookmark.port) : "22");
        setUsername(editBookmark.username ?? "");
        setCredentialProfileId(editBookmark.credential_profile_id ?? "");
      } else {
        setTitle("");
        setAssetId("");
        setHost("");
        setPort("22");
        setUsername("");
        setCredentialProfileId("");
      }
    }
    prevOpenRef.current = isOpen;
  }, [isOpen, editBookmark]);

  // Focus title input when dialog opens.
  useEffect(() => {
    if (isOpen) {
      const timer = window.setTimeout(() => titleInputRef.current?.focus(), 0);
      return () => window.clearTimeout(timer);
    }
  }, [isOpen]);

  // Close on Escape.
  useEffect(() => {
    if (!isOpen) return undefined;
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [isOpen, onClose]);

  const assetLinked = assetId.trim() !== "";

  const handleSubmit = useCallback(
    (event: React.FormEvent) => {
      event.preventDefault();
      const trimmedTitle = title.trim();
      if (!trimmedTitle) return;

      const parsedPort = parseInt(port, 10);
      const formData: BookmarkFormData = {
        title: trimmedTitle,
      };

      if (assetId.trim()) {
        formData.asset_id = assetId.trim();
      } else {
        if (host.trim()) formData.host = host.trim();
        formData.port =
          Number.isFinite(parsedPort) && parsedPort > 0 ? parsedPort : 22;
        if (username.trim()) formData.username = username.trim();
      }

      if (credentialProfileId.trim()) {
        formData.credential_profile_id = credentialProfileId.trim();
      }

      onSave(formData);
    },
    [title, assetId, host, port, username, credentialProfileId, onSave],
  );

  if (!isOpen) return null;

  const canSubmit = title.trim() !== "";

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={editBookmark ? "Edit Bookmark" : "New Bookmark"}
      className="fixed inset-0 z-[70] flex items-start justify-center p-4 pt-[8vh]"
    >
      <button
        type="button"
        aria-label="Close bookmark dialog"
        onClick={onClose}
        className="absolute inset-0 bg-black/76"
      />

      <div className="relative z-10 w-full max-w-md overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--panel)] shadow-[var(--shadow-lg)]">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[var(--line)] px-4 py-3">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
              {editBookmark ? "Edit Bookmark" : "New Bookmark"}
            </p>
            <p className="mt-0.5 text-sm font-semibold text-[var(--text)]">
              {editBookmark ? "Update saved connection" : "Save a connection"}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-6 w-6 items-center justify-center rounded-md border border-[var(--line)] bg-[var(--surface)] text-[var(--muted)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--text)]"
          >
            <X size={13} />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="space-y-4 p-4">
          {/* Title */}
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
              Title <span className="text-[var(--bad)]">*</span>
            </span>
            <input
              ref={titleInputRef}
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="My Server"
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
              autoComplete="off"
            />
          </label>

          {/* Asset ID */}
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
              Asset ID{" "}
              <span className="text-xs font-normal text-[var(--muted)]">
                (optional — links to a managed asset)
              </span>
            </span>
            <input
              type="text"
              value={assetId}
              onChange={(e) => setAssetId(e.target.value)}
              placeholder="asset-uuid"
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
              autoComplete="off"
            />
          </label>

          {/* Host + Port */}
          <div className="grid grid-cols-[1fr_5rem] gap-2">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                Host
              </span>
              <input
                type="text"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="192.168.1.10 or hostname"
                disabled={assetLinked}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50"
                autoComplete="off"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                Port
              </span>
              <input
                type="text"
                inputMode="numeric"
                value={port}
                onChange={(e) => setPort(e.target.value)}
                placeholder="22"
                disabled={assetLinked}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50"
              />
            </label>
          </div>

          {/* Username */}
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
              Username
            </span>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="root"
              disabled={assetLinked}
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50"
              autoComplete="off"
            />
          </label>

          {/* Credential Profile */}
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
              Credential Profile ID{" "}
              <span className="text-xs font-normal text-[var(--muted)]">
                (optional)
              </span>
            </span>
            <input
              type="text"
              value={credentialProfileId}
              onChange={(e) => setCredentialProfileId(e.target.value)}
              placeholder="profile-uuid"
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
              autoComplete="off"
            />
          </label>

          {/* Asset-linked hint */}
          {assetLinked && (
            <p className="text-xs text-[var(--muted)]">
              Host, port, and username are resolved from the linked asset.
            </p>
          )}

          {/* Actions */}
          <div className="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-sm text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
                canSubmit
                  ? "bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]"
                  : "cursor-not-allowed bg-[var(--surface)] text-[var(--muted)] opacity-60"
              }`}
            >
              {editBookmark ? "Update" : "Save"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
