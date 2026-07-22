"use client";

import { type FormEvent, useMemo, useState } from "react";
import { Eye, Play, Plus, RefreshCw, Trash2, X } from "lucide-react";
import { useTranslations } from "next-intl";

import type { Asset } from "../../../console/models";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input, Select } from "../../../components/ui/Input";
import { Modal } from "../../../components/ui/Modal";
import {
  type CreateSavedActionRequest,
  type SavedAction,
  type SavedActionRun,
  type SavedActionStep,
  useSavedActions,
} from "../../../hooks/useSavedActions";
import { sanitizeErrorMessage } from "../../../lib/sanitizeErrorMessage";

const maxSteps = 50;
const textareaClass = "w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 text-sm text-[var(--text)] outline-none transition-[border-color,box-shadow] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_var(--accent-subtle)] disabled:cursor-not-allowed disabled:text-[var(--text-disabled)]";

type SavedActionsCardProps = {
  assets: Asset[];
};

function blankStep(assetID = "", index = 0): SavedActionStep {
  return { name: `Step ${index + 1}`, command: "", target: assetID };
}

function errorText(cause: unknown, fallback: string): string {
  return sanitizeErrorMessage(cause instanceof Error ? cause.message : "", fallback);
}

function createdLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

export function SavedActionsCard({ assets }: SavedActionsCardProps) {
  const t = useTranslations("actions.savedActions");
  const {
    actions,
    loading,
    error,
    loadActions,
    createAction,
    getAction,
    runAction,
    deleteAction,
  } = useSavedActions();

  const sortedAssets = useMemo(
    () => [...assets].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id)),
    [assets],
  );
  const assetByID = useMemo(() => new Map(sortedAssets.map((asset) => [asset.id, asset])), [sortedAssets]);

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [steps, setSteps] = useState<SavedActionStep[]>([blankStep()]);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedAction, setSelectedAction] = useState<SavedAction | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [runPending, setRunPending] = useState(false);
  const [runResult, setRunResult] = useState<SavedActionRun | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<SavedAction | null>(null);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const [message, setMessage] = useState<string | null>(null);

  const openCreate = () => {
    setName("");
    setDescription("");
    setSteps([blankStep(sortedAssets[0]?.id ?? "")]);
    setCreateError(null);
    setCreateOpen(true);
  };

  const updateStep = (index: number, patch: Partial<SavedActionStep>) => {
    setSteps((current) => current.map((step, stepIndex) => stepIndex === index ? { ...step, ...patch } : step));
  };

  const submitCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalized: CreateSavedActionRequest = {
      name: name.trim(),
      ...(description.trim() ? { description: description.trim() } : {}),
      steps: steps.map((step, index) => ({
        name: step.name.trim() || t("defaultStepName", { number: index + 1 }),
        command: step.command.trim(),
        target: step.target,
      })),
    };
    if (!normalized.name) {
      setCreateError(t("errors.nameRequired"));
      return;
    }
    if (normalized.steps.some((step) => !step.command || !step.target)) {
      setCreateError(t("errors.stepRequired"));
      return;
    }
    if (normalized.steps.some((step) => !assetByID.has(step.target))) {
      setCreateError(t("errors.assetUnavailable"));
      return;
    }

    setCreatePending(true);
    setCreateError(null);
    try {
      const created = await createAction(normalized);
      setCreateOpen(false);
      setMessage(t("messages.created", { name: created.name }));
    } catch (cause) {
      setCreateError(errorText(cause, t("errors.createFailed")));
    } finally {
      setCreatePending(false);
    }
  };

  const showAction = async (action: SavedAction) => {
    setSelectedAction(null);
    setRunResult(null);
    setDetailError(null);
    setDetailOpen(true);
    setDetailLoading(true);
    try {
      setSelectedAction(await getAction(action.id));
    } catch (cause) {
      setSelectedAction(null);
      setDetailError(errorText(cause, t("errors.loadOneFailed")));
    } finally {
      setDetailLoading(false);
    }
  };

  const runSelectedAction = async (action: SavedAction) => {
    setSelectedAction(action);
    setRunResult(null);
    setDetailError(null);
    setDetailOpen(true);
    setRunPending(true);
    try {
      const result = await runAction(action.id);
      setRunResult(result);
      const failedSteps = result.steps.filter((step) => Boolean(step.error) || step.exit_code !== 0).length;
      setMessage(failedSteps > 0
        ? t("messages.runFinishedWithFailures", { name: action.name, count: failedSteps })
        : t("messages.runFinished", { name: action.name }));
    } catch (cause) {
      setSelectedAction(null);
      setDetailError(errorText(cause, t("errors.runFailed")));
    } finally {
      setRunPending(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget || deleteConfirmation !== deleteTarget.name) return;
    setDeletePending(true);
    setDeleteError(null);
    try {
      await deleteAction(deleteTarget.id);
      const deletedName = deleteTarget.name;
      setDeleteTarget(null);
      setDeleteConfirmation("");
      if (selectedAction?.id === deleteTarget.id) {
        setDetailOpen(false);
        setSelectedAction(null);
      }
      setMessage(t("messages.deleted", { name: deletedName }));
    } catch (cause) {
      setDeleteError(errorText(cause, t("errors.deleteFailed")));
    } finally {
      setDeletePending(false);
    }
  };

  return (
    <>
      <Card>
        <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t("heading")}</h2>
            <p className="mt-1 text-xs text-[var(--muted)]">{t("description")}</p>
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={loading}
              onClick={() => { void loadActions().catch(() => undefined); }}
              aria-label={t("reload")}
            >
              <RefreshCw size={13} aria-hidden="true" />
              {t("reload")}
            </Button>
            <Button type="button" size="sm" variant="primary" onClick={openCreate} disabled={sortedAssets.length === 0}>
              <Plus size={13} aria-hidden="true" />
              {t("create")}
            </Button>
          </div>
        </div>

        {sortedAssets.length === 0 ? (
          <p role="status" className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3 text-xs text-[var(--muted)]">
            {t("noAssets")}
          </p>
        ) : null}
        {error ? <p role="alert" className="mb-3 text-xs text-[var(--bad)]">{error}</p> : null}
        {message ? <p role="status" aria-live="polite" className="mb-3 text-xs text-[var(--good)]">{message}</p> : null}

        {loading && actions.length === 0 ? <p role="status" className="py-3 text-sm text-[var(--muted)]">{t("loading")}</p> : null}
        {!loading && !error && actions.length === 0 ? (
          <p className="rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">{t("empty")}</p>
        ) : null}
        {actions.length > 0 ? (
          <ul className="divide-y divide-[var(--line)]" aria-label={t("listLabel")}>
            {actions.map((action) => (
              <li key={action.id} className="flex flex-col gap-3 py-3 first:pt-1 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-[var(--text)]">{action.name}</p>
                  {action.description ? <p className="mt-0.5 line-clamp-2 text-xs text-[var(--muted)]">{action.description}</p> : null}
                  <p className="mt-1 text-[10px] font-mono text-[var(--muted)]">
                    {t("summary", { count: action.steps.length, date: createdLabel(action.created_at) })}
                  </p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Button type="button" size="sm" variant="ghost" onClick={() => { void showAction(action); }}>
                    <Eye size={13} aria-hidden="true" /> {t("view")}
                  </Button>
                  <Button type="button" size="sm" variant="secondary" onClick={() => { void runSelectedAction(action); }} disabled={runPending}>
                    <Play size={13} aria-hidden="true" /> {t("run")}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="danger"
                    onClick={() => {
                      setDeleteTarget(action);
                      setDeleteConfirmation("");
                      setDeleteError(null);
                    }}
                    aria-label={t("deleteAction", { name: action.name })}
                  >
                    <Trash2 size={13} aria-hidden="true" /> {t("delete")}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        ) : null}
      </Card>

      <Modal open={createOpen} onClose={() => { if (!createPending) setCreateOpen(false); }} title={t("createTitle")} className="md:max-w-3xl">
        <form onSubmit={submitCreate} className="max-h-[calc(100vh-7rem)] space-y-4 overflow-y-auto px-5 py-4">
          <label className="block space-y-1 text-xs text-[var(--muted)]">
            <span>{t("fields.name")}</span>
            <Input value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required autoFocus />
          </label>
          <label className="block space-y-1 text-xs text-[var(--muted)]">
            <span>{t("fields.description")}</span>
            <textarea className={textareaClass} value={description} onChange={(event) => setDescription(event.target.value)} maxLength={1000} rows={2} />
          </label>

          <fieldset className="space-y-3">
            <legend className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t("fields.steps")}</legend>
            {steps.map((step, index) => (
              <div key={index} className="space-y-3 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-medium text-[var(--text)]">{t("stepHeading", { number: index + 1 })}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={steps.length === 1}
                    onClick={() => setSteps((current) => current.filter((_, stepIndex) => stepIndex !== index))}
                    aria-label={t("removeStep", { number: index + 1 })}
                  >
                    <X size={13} aria-hidden="true" /> {t("remove")}
                  </Button>
                </div>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <label className="block space-y-1 text-xs text-[var(--muted)]">
                    <span>{t("fields.stepName")}</span>
                    <Input value={step.name} onChange={(event) => updateStep(index, { name: event.target.value })} maxLength={200} required />
                  </label>
                  <label className="block space-y-1 text-xs text-[var(--muted)]">
                    <span>{t("fields.asset")}</span>
                    <Select value={step.target} onChange={(event) => updateStep(index, { target: event.target.value })} required>
                      <option value="" disabled>{t("selectAsset")}</option>
                      {sortedAssets.map((asset) => (
                        <option key={asset.id} value={asset.id}>{asset.name || asset.id} ({asset.id})</option>
                      ))}
                    </Select>
                  </label>
                </div>
                <label className="block space-y-1 text-xs text-[var(--muted)]">
                  <span>{t("fields.command")}</span>
                  <textarea
                    className={`${textareaClass} font-mono`}
                    value={step.command}
                    onChange={(event) => updateStep(index, { command: event.target.value })}
                    maxLength={4096}
                    rows={3}
                    required
                    spellCheck={false}
                  />
                </label>
              </div>
            ))}
          </fieldset>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={steps.length >= maxSteps}
            onClick={() => setSteps((current) => [...current, blankStep(sortedAssets[0]?.id ?? "", current.length)])}
          >
            <Plus size={13} aria-hidden="true" /> {t("addStep")}
          </Button>
          {createError ? <p role="alert" className="text-xs text-[var(--bad)]">{createError}</p> : null}
          <div className="flex justify-end gap-2 border-t border-[var(--line)] pt-4">
            <Button type="button" variant="ghost" onClick={() => setCreateOpen(false)} disabled={createPending}>{t("cancel")}</Button>
            <Button type="submit" variant="primary" disabled={createPending || sortedAssets.length === 0}>
              {createPending ? t("creating") : t("createAction")}
            </Button>
          </div>
        </form>
      </Modal>

      <Modal open={detailOpen} onClose={() => { if (!runPending) setDetailOpen(false); }} title={selectedAction?.name ?? t("detailsTitle")} className="md:max-w-3xl">
        <div className="max-h-[calc(100vh-7rem)] space-y-4 overflow-y-auto px-5 py-4">
          {detailLoading ? <p role="status" className="text-sm text-[var(--muted)]">{t("loadingDetails")}</p> : null}
          {selectedAction ? (
            <>
              {selectedAction.description ? <p className="text-sm text-[var(--text-secondary)]">{selectedAction.description}</p> : null}
              <ol className="space-y-3" aria-label={t("stepListLabel")}>
                {selectedAction.steps.map((step, index) => {
                  const asset = assetByID.get(step.target);
                  return (
                    <li key={`${step.target}-${index}`} className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="text-sm font-medium text-[var(--text)]">{index + 1}. {step.name}</p>
                        <span className="text-[10px] font-mono text-[var(--muted)]">{asset?.name || step.target} ({step.target})</span>
                      </div>
                      <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-words rounded bg-black/20 p-2 text-xs text-[var(--text)]"><code>{step.command}</code></pre>
                    </li>
                  );
                })}
              </ol>
              <div className="flex justify-end">
                <Button type="button" variant="primary" onClick={() => { void runSelectedAction(selectedAction); }} disabled={runPending || detailLoading}>
                  <Play size={13} aria-hidden="true" /> {runPending ? t("running") : t("runNow")}
                </Button>
              </div>
            </>
          ) : null}
          {detailError ? <p role="alert" className="text-xs text-[var(--bad)]">{detailError}</p> : null}
          {runResult ? (
            <section aria-labelledby="saved-action-results-heading" className="space-y-3 border-t border-[var(--line)] pt-4">
              <h3 id="saved-action-results-heading" className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t("results")}</h3>
              <ol className="space-y-3">
                {runResult.steps.map((step, index) => {
                  const succeeded = !step.error && step.exit_code === 0;
                  return (
                    <li key={`${step.target}-${index}`} className="rounded-lg border border-[var(--line)] p-3">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="text-sm text-[var(--text)]">{index + 1}. {step.name}</p>
                        <span className="inline-flex items-center gap-1.5 text-xs text-[var(--muted)]">
                          <Badge status={succeeded ? "ok" : "bad"} size="sm" dot />
                          {step.error ? t("failed") : t("exitCode", { code: step.exit_code ?? 1 })}
                        </span>
                      </div>
                      <p className="mt-1 text-[10px] font-mono text-[var(--muted)]">
                        {step.target}{typeof step.duration_ms === "number" ? ` · ${step.duration_ms}ms` : ""}
                      </p>
                      {step.message ? <p role="alert" className="mt-2 text-xs text-[var(--bad)]">{step.message}</p> : null}
                      {typeof step.output === "string" ? (
                        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-black/20 p-2 text-xs text-[var(--text)]"><code>{step.output || t("noOutput")}</code></pre>
                      ) : null}
                    </li>
                  );
                })}
              </ol>
            </section>
          ) : null}
          <div className="flex justify-end border-t border-[var(--line)] pt-4">
            <Button type="button" variant="ghost" onClick={() => setDetailOpen(false)} disabled={runPending}>
              {t("close")}
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        open={deleteTarget !== null}
        onClose={() => { if (!deletePending) setDeleteTarget(null); }}
        title={t("deleteTitle")}
        className="md:max-w-md"
      >
        <div className="space-y-4 px-5 py-4">
          <p className="text-sm text-[var(--text-secondary)]">{t("deleteWarning", { name: deleteTarget?.name ?? "" })}</p>
          <label className="block space-y-1 text-xs text-[var(--muted)]">
            <span>{t("deletePrompt", { name: deleteTarget?.name ?? "" })}</span>
            <Input
              value={deleteConfirmation}
              onChange={(event) => setDeleteConfirmation(event.target.value)}
              autoComplete="off"
              autoFocus
            />
          </label>
          {deleteError ? <p role="alert" className="text-xs text-[var(--bad)]">{deleteError}</p> : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => setDeleteTarget(null)} disabled={deletePending}>{t("cancel")}</Button>
            <Button
              type="button"
              variant="danger"
              disabled={!deleteTarget || deleteConfirmation !== deleteTarget.name || deletePending}
              onClick={() => { void confirmDelete(); }}
            >
              <Trash2 size={13} aria-hidden="true" /> {deletePending ? t("deleting") : t("deleteActionButton")}
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
