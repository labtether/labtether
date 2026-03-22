"use client";

import { useRef, useState } from "react";
import { MoreHorizontal, Users } from "lucide-react";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import type { HubUser } from "../../../hooks/useHubUsers";

type UsersTableProps = {
  users: HubUser[];
  loading: boolean;
  currentUserId: string;
  onEditRole: (user: HubUser) => void;
  onResetPassword: (user: HubUser) => void;
  onRevokeSessions: (user: HubUser) => void;
  onDelete: (user: HubUser) => void;
};

const ROLE_STYLES: Record<string, string> = {
  owner: "bg-purple-500/15 text-purple-300 border border-purple-500/25",
  admin: "bg-red-500/15 text-red-300 border border-red-500/25",
  operator: "bg-[var(--accent)]/15 text-[var(--accent)] border border-[var(--accent)]/25",
  viewer: "bg-[var(--line)] text-[var(--muted)] border border-[var(--line)]",
};

function relativeTime(dateStr?: string): string {
  if (!dateStr) return "—";
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return "—";
  const diff = Date.now() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

function ActionsMenu({
  user,
  isCurrentUser,
  onEditRole,
  onResetPassword,
  onRevokeSessions,
  onDelete,
}: {
  user: HubUser;
  isCurrentUser: boolean;
  onEditRole: () => void;
  onResetPassword: () => void;
  onRevokeSessions: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const isOwner = user.role === "owner";
  const isLocal = !user.auth_provider || user.auth_provider === "local";

  const close = () => setOpen(false);

  return (
    <div className="relative">
      <button
        ref={buttonRef}
        type="button"
        className="p-1.5 rounded hover:bg-[var(--hover)] text-[var(--muted)] hover:text-[var(--text)] transition-colors"
        style={{ transitionDuration: "var(--dur-instant)" }}
        onClick={() => setOpen((v) => !v)}
        aria-label="User actions"
      >
        <MoreHorizontal size={14} />
      </button>

      {open ? (
        <>
          <div className="fixed inset-0 z-40" onClick={close} />
          <div className="absolute right-0 z-50 mt-1 w-44 rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] shadow-[var(--shadow-panel)] backdrop-blur-sm overflow-hidden">
            <button
              type="button"
              className="w-full text-left px-3 py-2 text-xs text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
              style={{ transitionDuration: "var(--dur-instant)" }}
              onClick={() => { close(); onEditRole(); }}
              disabled={isOwner}
            >
              Edit Role
            </button>
            {isLocal ? (
              <button
                type="button"
                className="w-full text-left px-3 py-2 text-xs text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
                style={{ transitionDuration: "var(--dur-instant)" }}
                onClick={() => { close(); onResetPassword(); }}
              >
                Reset Password
              </button>
            ) : null}
            <button
              type="button"
              className="w-full text-left px-3 py-2 text-xs text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
              style={{ transitionDuration: "var(--dur-instant)" }}
              onClick={() => { close(); onRevokeSessions(); }}
            >
              Revoke Sessions
            </button>
            {!isOwner && !isCurrentUser ? (
              <>
                <div className="border-t border-[var(--line)]" />
                <button
                  type="button"
                  className="w-full text-left px-3 py-2 text-xs text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors"
                  style={{ transitionDuration: "var(--dur-instant)" }}
                  onClick={() => { close(); onDelete(); }}
                >
                  Delete
                </button>
              </>
            ) : null}
          </div>
        </>
      ) : null}
    </div>
  );
}

export function UsersTable({
  users,
  loading,
  currentUserId,
  onEditRole,
  onResetPassword,
  onRevokeSessions,
  onDelete,
}: UsersTableProps) {
  return (
    <Card variant="flush">
      <div className="grid grid-cols-[minmax(0,1.5fr)_100px_80px_48px_90px_40px] gap-3 px-4 py-2.5 border-b border-[var(--line)] text-[10px] uppercase tracking-wide text-[var(--muted)] font-semibold">
        <span>Username</span>
        <span>Role</span>
        <span>Auth</span>
        <span>2FA</span>
        <span>Created</span>
        <span />
      </div>

      {loading ? (
        <div className="px-4 py-2 space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : users.length === 0 ? (
        <EmptyState
          icon={Users}
          title="No users found"
          description="Create the first user to get started."
        />
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {users.map((user) => {
            const isCurrentUser = user.id === currentUserId;
            const roleStyle = ROLE_STYLES[user.role] ?? ROLE_STYLES.viewer;
            const authLabel = !user.auth_provider || user.auth_provider === "local" ? "Local" : user.auth_provider;

            return (
              <li
                key={user.id}
                className="grid grid-cols-[minmax(0,1.5fr)_100px_80px_48px_90px_40px] gap-3 px-4 py-2.5 items-center hover:bg-[var(--hover)] transition-colors"
                style={{ transitionDuration: "var(--dur-instant)" }}
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm text-[var(--text)] truncate">{user.username}</span>
                  {isCurrentUser ? (
                    <span className="text-[10px] text-[var(--muted)] shrink-0">(you)</span>
                  ) : null}
                </div>

                <div>
                  <span className={`inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium capitalize ${roleStyle}`}>
                    {user.role}
                  </span>
                </div>

                <span className="text-xs text-[var(--muted)] truncate">{authLabel}</span>

                <div>
                  {user.totp_enabled ? (
                    <span
                      className="inline-block w-2 h-2 rounded-full bg-[var(--ok)]"
                      title="2FA enabled"
                    />
                  ) : (
                    <span
                      className="inline-block w-2 h-2 rounded-full bg-[var(--line)]"
                      title="2FA disabled"
                    />
                  )}
                </div>

                <span className="text-xs text-[var(--muted)]">{relativeTime(user.created_at)}</span>

                <div className="flex justify-end">
                  <ActionsMenu
                    user={user}
                    isCurrentUser={isCurrentUser}
                    onEditRole={() => onEditRole(user)}
                    onResetPassword={() => onResetPassword(user)}
                    onRevokeSessions={() => onRevokeSessions(user)}
                    onDelete={() => onDelete(user)}
                  />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}
