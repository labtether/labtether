"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useMemo, useState } from "react";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { useToast } from "../../../contexts/ToastContext";
import { useActions } from "../../../hooks/useActions";
import { type SavedAction, useSavedActions } from "../../../hooks/useSavedActions";
import { errorText } from "./_parts/autoApi";
import { AutoDialog } from "./_parts/AutoDialog";
import { ActionEditor, type EditorStart } from "./ActionEditor";
import { actionDevices, failedSteps, type SessionRun } from "./actionsModel";
import { RecentRuns } from "./RecentRuns";
import { SavedActionList } from "./SavedActionList";
import { useCanWrite } from "../../../hooks/useCanWrite";

/**
 * Automations › Actions: saved command actions (run, open for steps and the last run's output, duplicate, delete),
 * New action for a saved command or a one-off integration action (with a dry-run preview), and the hub's recent runs.
 */
export default function ActionsPage() {
  const t = useTranslations("actions");
  const tc = useTranslations("actions.common");
  const { addToast } = useToast();
  const integration = useActions();
  const saved = useSavedActions();
  const { assets } = integration;

  const names = useMemo(() => new Map(assets.map((a) => [a.id, a.name || a.id])), [assets]);
  const deviceName = useCallback((id: string) => names.get(id) ?? id, [names]);
  const actionNames = useMemo(
    () => new Map(integration.connectorActions.map((a) => [`${integration.selectedConnector}:${a.id}`, a.name])),
    [integration.connectorActions, integration.selectedConnector],
  );

  const [editor, setEditor] = useState<EditorStart | null>(null);
  const [runs, setRuns] = useState<Record<string, SessionRun>>({});
  const [openID, setOpenID] = useState<string | null>(null);
  const [openError, setOpenError] = useState<string | null>(null);
  const [confirmRun, setConfirmRun] = useState<SavedAction | null>(null);
  const [toDelete, setToDelete] = useState<SavedAction | null>(null);
  const [typed, setTyped] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const open = async (action: SavedAction | null) => {
    setOpenID(action?.id ?? null);
    setOpenError(null);
    if (!action) return;
    try {
      await saved.getAction(action.id);
    } catch (cause) {
      setOpenError(errorText(cause, t("panel.loadFailed")));
    }
  };

  const run = async (action: SavedAction) => {
    setConfirmRun(null);
    setOpenID(action.id);
    setRuns((all) => ({ ...all, [action.id]: { state: "running" } }));
    try {
      const result = await saved.runAction(action.id);
      const failed = failedSteps(result);
      setRuns((all) => ({ ...all, [action.id]: { state: "done", at: new Date().toISOString(), result, failed } }));
      addToast(failed ? "warning" : "success", failed
        ? t("run.finishedFailures", { name: action.name, count: failed })
        : t("run.finished", { name: action.name }));
    } catch (cause) {
      const message = errorText(cause, t("run.failed", { name: action.name }));
      setRuns((all) => ({ ...all, [action.id]: { state: "refused", at: new Date().toISOString(), message } }));
    }
  };

  const askRun = (action: SavedAction) => {
    if (actionDevices(action).length > 1) setConfirmRun(action);
    else void run(action);
  };

  const askDelete = (action: SavedAction) => {
    setToDelete(action);
    setTyped("");
    setDeleteError(null);
  };

  const remove = async () => {
    if (!toDelete || typed !== toDelete.name) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await saved.deleteAction(toDelete.id);
      if (openID === toDelete.id) setOpenID(null);
      addToast("success", t("delete.deleted", { name: toDelete.name }));
      setToDelete(null);
    } catch (cause) {
      setDeleteError(errorText(cause, t("delete.failed")));
    } finally {
      setDeleting(false);
    }
  };

  const runDevices = confirmRun ? actionDevices(confirmRun) : [];
  const canWrite = useCanWrite();

  return (
    <div className="au-page">
      <PageHeader
        title={t("title")}
        action={canWrite ? (
          <Button variant={editor ? "secondary" : "primary"} onClick={() => setEditor({ kind: "command" })}>
            <Plus size={14} aria-hidden="true" />
            {t("new")}
          </Button>
        ) : undefined}
      />
      {editor ? (
        <ActionEditor
          key={editor.from?.id ?? editor.kind}
          start={editor}
          assets={assets}
          integration={integration}
          createAction={saved.createAction}
          onClose={(created) => { setEditor(null); if (created) setOpenID(created.id); }}
        />
      ) : null}
      <SavedActionList
        actions={saved.actions}
        loading={saved.loading}
        error={saved.error}
        runs={runs}
        openID={openID}
        openError={openError}
        deviceName={deviceName}
        onOpen={(action) => { void open(action); }}
        onRun={askRun}
        canWrite={canWrite}
        onDuplicate={(action) => setEditor({ kind: "command", from: action })}
        onDelete={askDelete}
        onRetry={() => { void saved.loadActions().catch(() => undefined); }}
      />
      <RecentRuns runs={integration.actionRuns} connectors={integration.connectors} actionNames={actionNames} deviceName={deviceName} />

      <AutoDialog
        open={confirmRun !== null}
        title={t("run.title", { name: confirmRun?.name ?? "" })}
        cancelLabel={tc("cancel")}
        confirmLabel={t("run.confirm", { count: runDevices.length })}
        onConfirm={() => { if (confirmRun) void run(confirmRun); }}
        onClose={() => setConfirmRun(null)}
      >
        <p>{t("run.body", { count: runDevices.length, devices: runDevices.map(deviceName).join(", ") })}</p>
      </AutoDialog>

      <AutoDialog
        open={toDelete !== null}
        title={t("delete.title", { name: toDelete?.name ?? "" })}
        cancelLabel={tc("cancel")}
        confirmLabel={tc("delete")}
        danger
        pending={deleting}
        confirmDisabled={!toDelete || typed !== toDelete.name}
        error={deleteError}
        onConfirm={() => { void remove(); }}
        onClose={() => setToDelete(null)}
      >
        <p>{t("delete.body")}</p>
        <label className="au-field au-gap" htmlFor="au-del-name">
          <span>{t("delete.prompt", { name: toDelete?.name ?? "" })}</span>
          <Input id="au-del-name" value={typed} autoComplete="off" autoFocus onChange={(e) => setTyped(e.target.value)} />
        </label>
      </AutoDialog>
    </div>
  );
}
