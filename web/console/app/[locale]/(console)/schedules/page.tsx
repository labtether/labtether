"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Calendar, Pencil, Plus, Trash2 } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import { EmptyState } from "../../../components/ui/EmptyState";
import { apiFetch, apiMutate } from "../../../lib/api";

// ── Types ──

interface Schedule {
  id: string;
  name: string;
  cron_expr: string;
  command: string;
  targets: string[];
  group_id?: string;
  enabled: boolean;
  last_run?: string;
  next_run?: string;
}

interface SchedulesResponse {
  schedules: Schedule[];
}

// ── Helpers ──

function relativeTime(iso: string | undefined): string {
  if (!iso) return "";
  const delta = Date.now() - new Date(iso).getTime();
  const abs = Math.abs(delta);
  const future = delta < 0;
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

  if (abs < 60_000) return rtf.format(future ? Math.ceil(delta / 1000) : Math.floor(-delta / 1000), "second");
  if (abs < 3_600_000) return rtf.format(future ? Math.ceil(delta / 60_000) : Math.floor(-delta / 60_000), "minute");
  if (abs < 86_400_000) return rtf.format(future ? Math.ceil(delta / 3_600_000) : Math.floor(-delta / 3_600_000), "hour");
  return rtf.format(future ? Math.ceil(delta / 86_400_000) : Math.floor(-delta / 86_400_000), "day");
}

// ── Modal ──

type ModalMode = "create" | "edit";

interface ScheduleModalProps {
  mode: ModalMode;
  initial?: Schedule;
  saving: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (payload: {
    name: string;
    cron_expr: string;
    command: string;
    targets: string[];
    group_id?: string;
    enabled?: boolean;
  }) => void;
}

function ScheduleModal({ mode, initial, saving, error, onClose, onSubmit }: ScheduleModalProps) {
  const t = useTranslations("schedules");

  const [name, setName] = useState(initial?.name ?? "");
  const [cronExpr, setCronExpr] = useState(initial?.cron_expr ?? "");
  const [command, setCommand] = useState(initial?.command ?? "");
  const [targetsRaw, setTargetsRaw] = useState((initial?.targets ?? []).join(", "));
  const [groupId, setGroupId] = useState(initial?.group_id ?? "");

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !saving) {
        e.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [saving, onClose]);

  function handleSubmit() {
    const targets = targetsRaw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    onSubmit({
      name: name.trim(),
      cron_expr: cronExpr.trim(),
      command: command.trim(),
      targets,
      group_id: groupId.trim() || undefined,
    });
  }

  const title = mode === "create" ? t("createSchedule") : t("editSchedule");

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[34rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">{title}</h3>

          {/* Name */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("name")}</span>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("namePlaceholder")}
              disabled={saving}
            />
          </label>

          {/* Cron expression */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("cronExpr")}</span>
            <Input
              value={cronExpr}
              onChange={(e) => setCronExpr(e.target.value)}
              placeholder={t("cronPlaceholder")}
              disabled={saving}
              className="font-mono"
            />
            <span className="text-[10px] text-[var(--muted)]">{t("cronHelp")}</span>
          </label>

          {/* Command */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("command")}</span>
            <textarea
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder={t("commandPlaceholder")}
              disabled={saving}
              rows={3}
              className="w-full bg-transparent border border-[var(--line)] focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_var(--accent-subtle)] rounded-lg px-3 py-2 text-sm font-mono text-[var(--text)] placeholder:text-[var(--muted)] transition-[border-color,box-shadow] duration-[var(--dur-fast)] outline-none resize-none disabled:bg-[var(--surface)] disabled:text-[var(--text-disabled)] disabled:cursor-not-allowed"
            />
          </label>

          {/* Targets */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("targets")}</span>
            <Input
              value={targetsRaw}
              onChange={(e) => setTargetsRaw(e.target.value)}
              placeholder="asset-id-1, asset-id-2"
              disabled={saving}
            />
          </label>

          {/* Group */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("group")}</span>
            <Select
              value={groupId}
              onChange={(e) => setGroupId(e.target.value)}
              disabled={saving}
              className="w-full"
            >
              <option value="">— none —</option>
            </Select>
          </label>

          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}

          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={saving}>
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              onClick={handleSubmit}
              disabled={saving}
              loading={saving}
            >
              {t("save")}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

// ── Delete confirm ──

interface DeleteConfirmProps {
  schedule: Schedule;
  deleting: boolean;
  error: string;
  onClose: () => void;
  onConfirm: () => void;
}

function DeleteConfirm({ schedule, deleting, error, onClose, onConfirm }: DeleteConfirmProps) {
  const t = useTranslations("schedules");

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !deleting) {
        e.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [deleting, onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[26rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">{t("delete")} &ldquo;{schedule.name}&rdquo;</h3>
          <p className="text-sm text-[var(--muted)]">{t("deleteConfirm")}</p>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={deleting}>
              {t("cancel")}
            </Button>
            <Button variant="danger" onClick={onConfirm} disabled={deleting} loading={deleting}>
              {t("delete")}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

// ── Page ──

type ModalState =
  | { type: "create" }
  | { type: "edit"; schedule: Schedule }
  | { type: "delete"; schedule: Schedule }
  | null;

export default function SchedulesPage() {
  const t = useTranslations("schedules");

  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState("");

  const [modal, setModal] = useState<ModalState>(null);
  const [saving, setSaving] = useState(false);
  const [modalError, setModalError] = useState("");

  // ── Fetch ──

  const fetchSchedules = useCallback(async () => {
    setFetchError("");
    try {
      const { response, data } = await apiFetch<SchedulesResponse>("/api/v2/schedules");
      if (!response.ok) {
        setFetchError("Failed to load schedules.");
        return;
      }
      setSchedules(data?.schedules ?? []);
    } catch {
      setFetchError("Failed to load schedules.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchSchedules();
  }, [fetchSchedules]);

  // ── Create ──

  const handleCreate = useCallback(
    async (payload: {
      name: string;
      cron_expr: string;
      command: string;
      targets: string[];
      group_id?: string;
    }) => {
      setSaving(true);
      setModalError("");
      try {
        await apiMutate("/api/v2/schedules", "POST", payload);
        await fetchSchedules();
        setModal(null);
      } catch (err) {
        setModalError(err instanceof Error ? err.message : "Failed to create schedule.");
      } finally {
        setSaving(false);
      }
    },
    [fetchSchedules],
  );

  // ── Update ──

  const handleUpdate = useCallback(
    async (
      id: string,
      payload: {
        name: string;
        cron_expr: string;
        command: string;
        targets: string[];
        group_id?: string;
        enabled?: boolean;
      },
    ) => {
      setSaving(true);
      setModalError("");
      try {
        await apiMutate(`/api/v2/schedules/${id}`, "PATCH", payload);
        await fetchSchedules();
        setModal(null);
      } catch (err) {
        setModalError(err instanceof Error ? err.message : "Failed to update schedule.");
      } finally {
        setSaving(false);
      }
    },
    [fetchSchedules],
  );

  // ── Toggle enabled ──

  const handleToggleEnabled = useCallback(
    async (schedule: Schedule) => {
      try {
        await apiMutate(`/api/v2/schedules/${schedule.id}`, "PATCH", {
          enabled: !schedule.enabled,
        });
        await fetchSchedules();
      } catch {
        /* toggle failures are transient; page will reflect server state on next fetch */
      }
    },
    [fetchSchedules],
  );

  // ── Delete ──

  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");

  const handleDelete = useCallback(
    async (id: string) => {
      setDeleting(true);
      setDeleteError("");
      try {
        await apiMutate(`/api/v2/schedules/${id}`, "DELETE");
        await fetchSchedules();
        setModal(null);
      } catch (err) {
        setDeleteError(err instanceof Error ? err.message : "Failed to delete schedule.");
      } finally {
        setDeleting(false);
      }
    },
    [fetchSchedules],
  );

  // ── Modal helpers ──

  const openCreate = useCallback(() => {
    setModalError("");
    setModal({ type: "create" });
  }, []);

  const openEdit = useCallback((schedule: Schedule) => {
    setModalError("");
    setModal({ type: "edit", schedule });
  }, []);

  const openDelete = useCallback((schedule: Schedule) => {
    setDeleteError("");
    setModal({ type: "delete", schedule });
  }, []);

  const closeModal = useCallback(() => {
    if (saving || deleting) return;
    setModal(null);
    setModalError("");
    setDeleteError("");
  }, [saving, deleting]);

  // ── Render ──

  return (
    <>
      <PageHeader
        title={t("title")}
        subtitle={t("subtitle")}
        action={(
          <Button variant="primary" size="sm" onClick={openCreate}>
            <Plus size={14} />
            {t("createSchedule")}
          </Button>
        )}
      />

      <Card variant="flush">
        {loading ? (
          <div className="p-4">
            <EmptyState
              icon={Calendar}
              title="Loading..."
              description=""
            />
          </div>
        ) : fetchError ? (
          <div className="p-4">
            <p className="text-sm text-[var(--bad)]">{fetchError}</p>
          </div>
        ) : schedules.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={Calendar}
              title={t("noSchedules")}
              description={t("noSchedulesDesc")}
              action={(
                <Button size="sm" variant="secondary" onClick={openCreate}>
                  <Plus size={14} />
                  {t("createSchedule")}
                </Button>
              )}
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="px-4 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)]">
                    {t("name")}
                  </th>
                  <th className="px-4 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)]">
                    {t("cronExpr")}
                  </th>
                  <th className="px-4 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden md:table-cell">
                    {t("command")}
                  </th>
                  <th className="px-4 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden lg:table-cell">
                    {t("targets")}
                  </th>
                  <th className="px-4 py-2.5 text-center text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)]">
                    {t("enabled")}
                  </th>
                  <th className="px-4 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden lg:table-cell">
                    {t("lastRun")}
                  </th>
                  <th className="px-4 py-2.5 text-left text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden lg:table-cell">
                    {t("nextRun")}
                  </th>
                  <th className="px-4 py-2.5 text-right text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)]">
                    {/* actions */}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {schedules.map((s) => (
                  <tr
                    key={s.id}
                    className="hover:bg-[var(--hover)] transition-colors"
                    style={{ transitionDuration: "var(--dur-instant)" }}
                  >
                    {/* Name */}
                    <td className="px-4 py-3 font-medium text-[var(--text)]">
                      {s.name}
                    </td>

                    {/* Cron */}
                    <td className="px-4 py-3 font-mono text-xs text-[var(--muted)]">
                      {s.cron_expr}
                    </td>

                    {/* Command */}
                    <td className="px-4 py-3 text-xs text-[var(--muted)] hidden md:table-cell max-w-[200px]">
                      <span className="block truncate font-mono">{s.command}</span>
                    </td>

                    {/* Targets */}
                    <td className="px-4 py-3 hidden lg:table-cell">
                      {s.targets.length === 0 ? (
                        <span className="text-xs text-[var(--muted)]">{t("allDevices")}</span>
                      ) : (
                        <span className="inline-flex items-center rounded-full border border-[var(--control-border)] px-2 py-0.5 text-[10px] text-[var(--control-fg-muted)]">
                          {t("targetsCount", { count: s.targets.length })}
                        </span>
                      )}
                    </td>

                    {/* Enabled toggle */}
                    <td className="px-4 py-3 text-center">
                      <button
                        type="button"
                        aria-label={s.enabled ? "Disable schedule" : "Enable schedule"}
                        onClick={() => { void handleToggleEnabled(s); }}
                        className="inline-flex items-center justify-center w-8 h-5 rounded-full transition-colors cursor-pointer"
                        style={{ transitionDuration: "var(--dur-fast)" }}
                      >
                        <span
                          className={`block w-2 h-2 rounded-full ${
                            s.enabled
                              ? "bg-[var(--good)] shadow-[0_0_6px_var(--good)]"
                              : "bg-[var(--muted)]"
                          }`}
                        />
                      </button>
                    </td>

                    {/* Last run */}
                    <td className="px-4 py-3 text-xs text-[var(--muted)] hidden lg:table-cell">
                      {s.last_run ? relativeTime(s.last_run) : t("never")}
                    </td>

                    {/* Next run */}
                    <td className="px-4 py-3 text-xs text-[var(--muted)] hidden lg:table-cell">
                      {s.next_run ? relativeTime(s.next_run) : t("never")}
                    </td>

                    {/* Actions */}
                    <td className="px-4 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <button
                          type="button"
                          aria-label="Edit schedule"
                          onClick={() => openEdit(s)}
                          className="p-1.5 rounded text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
                          style={{ transitionDuration: "var(--dur-instant)" }}
                        >
                          <Pencil size={13} />
                        </button>
                        <button
                          type="button"
                          aria-label="Delete schedule"
                          onClick={() => openDelete(s)}
                          className="p-1.5 rounded text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors cursor-pointer"
                          style={{ transitionDuration: "var(--dur-instant)" }}
                        >
                          <Trash2 size={13} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Create modal */}
      {modal?.type === "create" ? (
        <ScheduleModal
          mode="create"
          saving={saving}
          error={modalError}
          onClose={closeModal}
          onSubmit={(payload) => { void handleCreate(payload); }}
        />
      ) : null}

      {/* Edit modal */}
      {modal?.type === "edit" ? (
        <ScheduleModal
          mode="edit"
          initial={modal.schedule}
          saving={saving}
          error={modalError}
          onClose={closeModal}
          onSubmit={(payload) => { void handleUpdate(modal.schedule.id, payload); }}
        />
      ) : null}

      {/* Delete confirm */}
      {modal?.type === "delete" ? (
        <DeleteConfirm
          schedule={modal.schedule}
          deleting={deleting}
          error={deleteError}
          onClose={closeModal}
          onConfirm={() => { void handleDelete(modal.schedule.id); }}
        />
      ) : null}
    </>
  );
}
