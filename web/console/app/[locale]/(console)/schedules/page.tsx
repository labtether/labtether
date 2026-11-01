"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useMemo, useState } from "react";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { useFastStatus, useSlowStatus } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { errorText } from "../actions/_parts/autoApi";
import { AutoDialog } from "../actions/_parts/AutoDialog";
import { ScheduleEditor } from "./ScheduleEditor";
import { ScheduleList } from "./ScheduleList";
import type { Schedule, ScheduleInput } from "./scheduleModel";
import { useSchedules } from "./useSchedules";

type EditorState = { mode: "new" } | { mode: "edit"; schedule: Schedule };

/** Automations › Schedules: commands the hub runs on a cron line, on devices or a group. */
export default function SchedulesPage() {
  const t = useTranslations("schedules");
  const tc = useTranslations("actions.common");
  const { addToast } = useToast();
  const assets = useFastStatus()?.assets;
  const groups = useSlowStatus()?.groups;
  const devices = useMemo(() => [...(assets ?? [])].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id)), [assets]);
  const names = useMemo(() => new Map(devices.map((d) => [d.id, d.name || d.id])), [devices]);
  const groupNames = useMemo(() => new Map((groups ?? []).map((g) => [g.id, g.name])), [groups]);
  const deviceName = useCallback((id: string) => names.get(id) ?? id, [names]);
  const groupName = useCallback((id: string) => groupNames.get(id) ?? id, [groupNames]);

  const { schedules, loading, failed, load, create, update, remove, setEnabled } = useSchedules();
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [toDelete, setToDelete] = useState<Schedule | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const save = async (input: ScheduleInput) => {
    if (editor?.mode === "edit") {
      await update(editor.schedule.id, input);
      addToast("success", t("saved", { name: input.name }));
    } else {
      await create(input);
      addToast("success", t("created", { name: input.name }));
    }
    setEditor(null);
  };

  const toggle = (schedule: Schedule, on: boolean) => {
    setEnabled(schedule, on).catch((cause: unknown) => {
      addToast("error", errorText(cause, t("toggleFailed", { name: schedule.name })));
    });
  };

  const confirmDelete = async () => {
    if (!toDelete) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await remove(toDelete.id);
      if (editor?.mode === "edit" && editor.schedule.id === toDelete.id) setEditor(null);
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
        action={(
          <Button variant={editor ? "secondary" : "primary"} onClick={() => setEditor({ mode: "new" })}>
            <Plus size={14} aria-hidden="true" />
            {t("new")}
          </Button>
        )}
      />
      {editor ? (
        <ScheduleEditor
          key={editor.mode === "edit" ? editor.schedule.id : "new"}
          schedule={editor.mode === "edit" ? editor.schedule : undefined}
          devices={devices}
          groups={groups ?? []}
          onSave={save}
          onClose={() => setEditor(null)}
        />
      ) : null}
      <ScheduleList
        schedules={schedules}
        loading={loading}
        failed={failed}
        deviceName={deviceName}
        groupName={groupName}
        onEdit={(schedule) => setEditor({ mode: "edit", schedule })}
        onToggle={toggle}
        onDelete={(schedule) => { setToDelete(schedule); setDeleteError(null); }}
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
