"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { useToast } from "../../../contexts/ToastContext";
import { errorText } from "../actions/_parts/autoApi";
import { AutoDialog } from "../actions/_parts/AutoDialog";
import { type WebhookInput, useWebhooks } from "./useWebhooks";
import { WebhookEditor } from "./WebhookEditor";
import { WebhookList } from "./WebhookList";
import type { WebhookRecord } from "./webhookPayload";
import { useCanWrite } from "../../../hooks/useCanWrite";

type EditorState = { mode: "new" } | { mode: "edit"; webhook: WebhookRecord };

/** Automations › Webhooks: addresses the hub posts events to, signed with a secret when one is set. */
export default function WebhooksPage() {
  const t = useTranslations("webhooks");
  const canWrite = useCanWrite();
  const tc = useTranslations("actions.common");
  const { addToast } = useToast();
  const { webhooks, loading, failed, load, create, update, remove, setEnabled } = useWebhooks();
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [toDelete, setToDelete] = useState<WebhookRecord | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const save = async (input: WebhookInput) => {
    if (editor?.mode === "edit") {
      await update(editor.webhook, input);
      addToast("success", t("saved", { name: input.name }));
    } else {
      await create(input);
      addToast("success", t("created", { name: input.name }));
    }
    setEditor(null);
  };

  const toggle = (webhook: WebhookRecord, on: boolean) => {
    setEnabled(webhook, on).catch((cause: unknown) => {
      addToast("error", errorText(cause, t("toggleFailed", { name: webhook.name })));
    });
  };

  const confirmDelete = async () => {
    if (!toDelete) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await remove(toDelete.id);
      if (editor?.mode === "edit" && editor.webhook.id === toDelete.id) setEditor(null);
      addToast("success", t("deleted", { name: toDelete.name }));
      setToDelete(null);
    } catch (cause) {
      setDeleteError(errorText(cause, t("deleteFailed")));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="au-page">
      <PageHeader
        title={t("title")}
        action={canWrite ? (
          <Button variant={editor ? "secondary" : "primary"} onClick={() => setEditor({ mode: "new" })}>
            <Plus size={14} aria-hidden="true" />
            {t("new")}
          </Button>
        ) : undefined}
      />
      {editor ? (
        <WebhookEditor
          key={editor.mode === "edit" ? editor.webhook.id : "new"}
          webhook={editor.mode === "edit" ? editor.webhook : undefined}
          onSave={save}
          onClose={() => setEditor(null)}
        />
      ) : null}
      <WebhookList
        webhooks={webhooks}
        loading={loading}
        failed={failed}
        onEdit={(webhook) => setEditor({ mode: "edit", webhook })}
        onToggle={toggle}
        onDelete={(webhook) => { setToDelete(webhook); setDeleteError(null); }}
        onRetry={() => { void load(); }}
      />
      <AutoDialog
        open={toDelete !== null}
        title={t("deleteTitle", { name: toDelete?.name ?? "" })}
        cancelLabel={tc("cancel")}
        confirmLabel={tc("delete")}
        danger
        pending={deleting}
        error={deleteError}
        onConfirm={() => { void confirmDelete(); }}
        onClose={() => setToDelete(null)}
      >
        <p>{t("deleteBody")}</p>
      </AutoDialog>
    </div>
  );
}
