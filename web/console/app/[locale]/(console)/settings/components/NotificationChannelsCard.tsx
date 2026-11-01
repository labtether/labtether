"use client";

import { useEffect, useRef, useState } from "react";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../../components/PageHeader";
import { Button } from "../../../../components/ui/Button";
import { useNotificationChannels } from "../../../../hooks/useNotificationChannels";
import type { NotificationChannel } from "../../../../hooks/useNotificationChannels";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";
import { AddChannelDialog } from "./AddChannelDialog";
import { EditChannelDialog } from "./EditChannelDialog";
import { channelTarget } from "./notificationChannelForm";
import { RowMenu } from "./RowMenu";
import { Message, SettingsDialog, Sheet, Status, Switch } from "./SettingsKit";

type TestState = { id: string; status: "sending" | "ok" | "error"; message?: string } | null;

type Dialog =
  | { type: "add" }
  | { type: "edit"; channel: NotificationChannel }
  | { type: "deleteConfirm"; channel: NotificationChannel }
  | null;

const KNOWN_TYPES = new Set(["slack", "email", "webhook", "ntfy", "gotify"]);

type ChannelRowProps = {
  channel: NotificationChannel;
  onToggle: (enabled: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
  onTest: () => void;
  testState: TestState;
};

function ChannelRow({ channel, onToggle, onEdit, onDelete, onTest, testState }: ChannelRowProps) {
  const t = useTranslations("notifications");
  const isSending = testState?.id === channel.id && testState.status === "sending";
  const testResult = testState?.id === channel.id && testState.status !== "sending" ? testState : null;
  const typeName = KNOWN_TYPES.has(channel.type) ? t(`typeSelect.${channel.type}.name` as Parameters<typeof t>[0]) : channel.type;
  const target = channelTarget(channel.type, channel.config ?? {});

  return (
    <div className="st-row st-ch">
      <div className="st-row-l">
        <span className="st-nm">{channel.name}</span>
        {testResult ? (
          <span role="status" aria-live="polite" className={`st-g${testResult.status === "error" ? " bad" : ""}`}>
            {testResult.status === "ok" ? t("testSent") : `${t("testFailed")}${testResult.message ? `: ${testResult.message}` : ""}`}
          </span>
        ) : (
          <span className="st-g">{target ? `${typeName} · ${target}` : typeName}</span>
        )}
      </div>
      <span className="st-ch-test">
        {isSending ? (
          <Status tone="busy">{t("sending")}</Status>
        ) : (
          <Button variant="ghost" size="sm" onClick={onTest} disabled={!channel.enabled}>
            {t("test")}
          </Button>
        )}
      </span>
      <Switch checked={channel.enabled} onChange={onToggle} label={t("sendTo", { name: channel.name })} />
      <RowMenu
        label={t("actionsFor", { name: channel.name })}
        items={[
          { label: t("edit"), onSelect: onEdit },
          { label: t("delete"), onSelect: onDelete, danger: true, separatorBefore: true },
        ]}
      />
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
      setError(err instanceof Error ? err.message : t("deleteDialog.failed"));
      setDeleting(false);
    }
  };

  return (
    <SettingsDialog labelledBy="delete-notification-channel-title" onClose={() => { if (!deleting) onClose(); }}>
      <h3 id="delete-notification-channel-title">{t("deleteDialog.title", { name: channel.name })}</h3>
      <p>{t("deleteDialog.body", { name: channel.name })}</p>
      {error ? <Message tone="bad">{error}</Message> : null}
      <div className="st-dlg-acts">
        <Button onClick={onClose} disabled={deleting}>{t("cancel")}</Button>
        <Button variant="danger" className="st-danger-fill" loading={deleting} onClick={() => { void handleConfirm(); }}>
          {t("deleteDialog.confirm")}
        </Button>
      </div>
    </SettingsDialog>
  );
}

/** The Notifications page body: its header (with Add channel) and the channel list. */
export function NotificationChannelsCard() {
  const t = useTranslations("notifications");
  const { channels, capabilities, loading, error, createChannel, updateChannel, deleteChannel, toggleEnabled, testChannel } = useNotificationChannels();
  const [dialog, setDialog] = useState<Dialog>(null);
  const [testState, setTestState] = useState<TestState>(null);
  const [actionError, setActionError] = useState("");
  const testResetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (testResetTimer.current) clearTimeout(testResetTimer.current);
  }, []);

  const handleToggle = async (channel: NotificationChannel, enabled: boolean) => {
    setActionError("");
    try {
      await toggleEnabled(channel.id, enabled);
    } catch (err) {
      setActionError(sanitizeErrorMessage(err instanceof Error ? err.message : "", t("toggleFailed")));
    }
  };

  const handleTest = async (channel: NotificationChannel) => {
    setTestState({ id: channel.id, status: "sending" });
    let result: { success: boolean; error?: string };
    try {
      result = await testChannel(channel.id);
    } catch (err) {
      result = { success: false, error: sanitizeErrorMessage(err instanceof Error ? err.message : "", t("testFailed")) };
    }
    setTestState({ id: channel.id, status: result.success ? "ok" : "error", message: result.error });
    if (testResetTimer.current) clearTimeout(testResetTimer.current);
    testResetTimer.current = setTimeout(() => {
      setTestState((prev) => (prev?.id === channel.id ? null : prev));
    }, 5000);
  };

  return (
    <>
      <PageHeader
        title={t("title")}
        action={(
          <Button variant="primary" onClick={() => setDialog({ type: "add" })}>
            <Plus size={14} aria-hidden="true" />
            {t("addChannel")}
          </Button>
        )}
      />

      <div className="st">
        <Sheet className={loading && channels.length === 0 ? "st-loading" : ""}>
          {channels.map((channel) => (
            <ChannelRow
              key={channel.id}
              channel={channel}
              onToggle={(enabled) => { void handleToggle(channel, enabled); }}
              onEdit={() => setDialog({ type: "edit", channel })}
              onDelete={() => setDialog({ type: "deleteConfirm", channel })}
              onTest={() => { void handleTest(channel); }}
              testState={testState}
            />
          ))}
          {!loading && !error && channels.length === 0 ? (
            <div className="st-empty">
              <b>{t("empty")}</b>
              <span>{t("emptyHint")}</span>
            </div>
          ) : null}
          {error ? <Message tone="bad">{error}</Message> : null}
          {actionError ? <Message tone="bad">{actionError}</Message> : null}
        </Sheet>
      </div>

      <AddChannelDialog
        open={dialog?.type === "add"}
        allowInsecureSMTP={capabilities.smtp_insecure_transport_allowed}
        onClose={() => setDialog(null)}
        onConfirm={createChannel}
      />

      <EditChannelDialog
        channel={dialog?.type === "edit" ? dialog.channel : null}
        allowInsecureSMTP={capabilities.smtp_insecure_transport_allowed}
        onClose={() => setDialog(null)}
        onConfirm={updateChannel}
      />

      {dialog?.type === "deleteConfirm" ? (
        <DeleteConfirmDialog
          channel={dialog.channel}
          onClose={() => setDialog(null)}
          onConfirm={async () => {
            await deleteChannel(dialog.channel.id);
            setDialog(null);
          }}
        />
      ) : null}
    </>
  );
}
