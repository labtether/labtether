"use client";

import { useState } from "react";
import { Bell, Globe, Mail, MessageSquare, MoreHorizontal, Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { useNotificationChannels } from "../../../../hooks/useNotificationChannels";
import type { NotificationChannel } from "../../../../hooks/useNotificationChannels";
import { AddChannelDialog } from "./AddChannelDialog";
import { EditChannelDialog } from "./EditChannelDialog";

type TestState = { id: string; status: "sending" | "ok" | "error"; message?: string } | null;

type Dialog =
  | { type: "add" }
  | { type: "edit"; channel: NotificationChannel }
  | { type: "deleteConfirm"; channel: NotificationChannel }
  | null;

function channelIcon(type: string) {
  switch (type) {
    case "slack":
      return <MessageSquare size={15} className="shrink-0 text-[var(--muted)]" />;
    case "email":
      return <Mail size={15} className="shrink-0 text-[var(--muted)]" />;
    case "webhook":
      return <Globe size={15} className="shrink-0 text-[var(--muted)]" />;
    case "ntfy":
    case "gotify":
    default:
      return <Bell size={15} className="shrink-0 text-[var(--muted)]" />;
  }
}

type ChannelRowProps = {
  channel: NotificationChannel;
  onToggle: (enabled: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
  onTest: () => void;
  menuOpen: boolean;
  onMenuToggle: () => void;
  testState: TestState;
};

function ChannelRow({ channel, onToggle, onEdit, onDelete, onTest, menuOpen, onMenuToggle, testState }: ChannelRowProps) {
  const t = useTranslations("notifications");
  const isSending = testState?.id === channel.id && testState.status === "sending";
  const testResult = testState?.id === channel.id && testState.status !== "sending" ? testState : null;

  return (
    <div className="flex items-center gap-3 px-3 py-2.5 border-t border-[var(--line)]">
      {channelIcon(channel.type)}
      <div className="min-w-0 flex-1">
        <p className="text-sm text-[var(--text)] truncate">{channel.name}</p>
        {testResult && (
          <p className={`text-xs mt-0.5 ${testResult.status === "ok" ? "text-[var(--good)]" : "text-[var(--bad)]"}`}>
            {testResult.status === "ok" ? t("testSent") : `${t("testFailed")}${testResult.message ? `: ${testResult.message}` : ""}`}
          </p>
        )}
      </div>
      <span className="text-xs text-[var(--muted)] shrink-0">{channel.type}</span>
      <button
        role="switch"
        aria-checked={channel.enabled}
        aria-label={channel.enabled ? t("enabled") : t("disabled")}
        onClick={() => onToggle(!channel.enabled)}
        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] ${
          channel.enabled ? "bg-[var(--accent)]" : "bg-[var(--line)]"
        }`}
      >
        <span
          className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm transition-transform duration-200 ${
            channel.enabled ? "translate-x-4" : "translate-x-0"
          }`}
        />
      </button>
      <div className="relative shrink-0">
        <button
          onClick={onMenuToggle}
          className="flex items-center justify-center h-7 w-7 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
          aria-label="Channel actions"
        >
          <MoreHorizontal size={15} />
        </button>
        {menuOpen && (
          <div className="absolute right-0 top-full z-10 mt-1 w-36 rounded-lg border border-[var(--line)] bg-[var(--panel)] shadow-[var(--shadow-panel)] py-1">
            <button
              className="w-full px-3 py-1.5 text-left text-xs text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none disabled:opacity-50 disabled:cursor-not-allowed"
              onClick={onTest}
              disabled={isSending}
            >
              {isSending ? "..." : t("test")}
            </button>
            <button
              className="w-full px-3 py-1.5 text-left text-xs text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
              onClick={onEdit}
            >
              {t("edit")}
            </button>
            <button
              className="w-full px-3 py-1.5 text-left text-xs text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
              onClick={onDelete}
            >
              {t("delete")}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

type DeleteConfirmDialogProps = {
  channel: NotificationChannel;
  onClose: () => void;
  onConfirm: () => Promise<void>;
};

function DeleteConfirmDialog({ channel, onClose, onConfirm }: DeleteConfirmDialogProps) {
  const t = useTranslations("notifications");
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");

  const handleConfirm = async () => {
    setDeleting(true);
    setError("");
    try {
      await onConfirm();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete channel.");
      setDeleting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={() => { if (!deleting) onClose(); }}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[28rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">{t("deleteDialog.title")}</h3>
          <p className="text-xs text-[var(--muted)]">
            {t("deleteDialog.body", { name: channel.name })}
          </p>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={deleting}>{t("cancel")}</Button>
            <Button variant="danger" loading={deleting} onClick={() => { void handleConfirm(); }}>{t("deleteDialog.confirm")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

export function NotificationChannelsCard() {
  const t = useTranslations("notifications");
  const { channels, loading, error, createChannel, updateChannel, deleteChannel, toggleEnabled, testChannel } = useNotificationChannels();
  const [dialog, setDialog] = useState<Dialog>(null);
  const [openMenuId, setOpenMenuId] = useState<string | null>(null);
  const [testState, setTestState] = useState<TestState>(null);

  const handleToggle = async (channel: NotificationChannel, enabled: boolean) => {
    try {
      await toggleEnabled(channel.id, enabled);
    } catch {
      // toggle failure is silent — state reverts on refresh
    }
  };

  const handleMenuToggle = (id: string) => {
    setOpenMenuId((prev) => (prev === id ? null : id));
  };

  const handleEdit = (channel: NotificationChannel) => {
    setOpenMenuId(null);
    setDialog({ type: "edit", channel });
  };

  const handleDeleteConfirm = (channel: NotificationChannel) => {
    setOpenMenuId(null);
    setDialog({ type: "deleteConfirm", channel });
  };

  const handleTest = async (channel: NotificationChannel) => {
    setOpenMenuId(null);
    setTestState({ id: channel.id, status: "sending" });
    const result = await testChannel(channel.id);
    setTestState({ id: channel.id, status: result.success ? "ok" : "error", message: result.error });
    setTimeout(() => {
      setTestState((prev) => (prev?.id === channel.id ? null : prev));
    }, 5000);
  };

  return (
    <>
      <Card className="mb-6">
        <div className="flex items-center justify-between mb-3">
          <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t("heading")}</p>
          <Button variant="secondary" size="sm" onClick={() => setDialog({ type: "add" })}>
            <Plus size={13} />
            {t("addChannel")}
          </Button>
        </div>

        {loading && (
          <p className="text-xs text-[var(--muted)] py-2">&nbsp;</p>
        )}

        {!loading && error && (
          <p className="text-xs text-[var(--bad)]">{error}</p>
        )}

        {!loading && !error && channels.length === 0 && (
          <p className="text-xs text-[var(--muted)] py-1">{t("empty")}</p>
        )}

        {!loading && channels.length > 0 && (
          <div className="border border-[var(--line)] rounded-lg overflow-hidden -mx-0">
            {channels.map((channel) => (
              <ChannelRow
                key={channel.id}
                channel={channel}
                onToggle={(enabled) => { void handleToggle(channel, enabled); }}
                onEdit={() => handleEdit(channel)}
                onDelete={() => handleDeleteConfirm(channel)}
                onTest={() => { void handleTest(channel); }}
                menuOpen={openMenuId === channel.id}
                onMenuToggle={() => handleMenuToggle(channel.id)}
                testState={testState}
              />
            ))}
          </div>
        )}
      </Card>

      <AddChannelDialog
        open={dialog?.type === "add"}
        onClose={() => setDialog(null)}
        onConfirm={createChannel}
      />

      <EditChannelDialog
        channel={dialog?.type === "edit" ? dialog.channel : null}
        onClose={() => setDialog(null)}
        onConfirm={updateChannel}
      />

      {dialog?.type === "deleteConfirm" && (
        <DeleteConfirmDialog
          channel={dialog.channel}
          onClose={() => setDialog(null)}
          onConfirm={() => deleteChannel(dialog.channel.id)}
        />
      )}
    </>
  );
}
