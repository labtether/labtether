"use client";

import { Plus, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import type { Asset } from "../../../console/models";
import { useToast } from "../../../contexts/ToastContext";
import type { useActions } from "../../../hooks/useActions";
import type { CreateSavedActionRequest, SavedAction } from "../../../hooks/useSavedActions";
import { Input, Select, Textarea } from "../../../components/ui/Input";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { errorText } from "./_parts/autoApi";
import { EditorSheet, Field, fieldAria } from "./_parts/Editor";
import { IntegrationFields, integrationErrors } from "./IntegrationFields";

const MAX_STEPS = 50;
type StepDraft = { name: string; target: string; command: string };
export type EditorStart = { kind: "command" | "integration"; from?: SavedAction };

/**
 * New action: a Command (saved: steps, each a device and a shell command its agent runs) or an Integration action (one
 * connector action with its parameters and a Preview switch for a dry run; the hub runs it once and keeps no copy).
 * Duplicate opens the Command form filled from an existing action.
 */
export function ActionEditor({
  start,
  assets,
  integration,
  createAction,
  onClose,
}: {
  start: EditorStart;
  assets: Asset[];
  integration: ReturnType<typeof useActions>;
  createAction: (request: CreateSavedActionRequest) => Promise<SavedAction>;
  onClose: (created?: SavedAction) => void;
}) {
  const t = useTranslations("actions.editor");
  const ti = useTranslations("actions.integration");
  const tc = useTranslations("actions.common");
  const { addToast } = useToast();
  const devices = useMemo(() => [...assets].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id)), [assets]);
  const known = useMemo(() => new Set(devices.map((d) => d.id)), [devices]);

  const [kind, setKind] = useState(start.kind);
  const [name, setName] = useState(start.from ? t("copyName", { name: start.from.name }) : "");
  const [description, setDescription] = useState(start.from?.description ?? "");
  const [steps, setSteps] = useState<StepDraft[]>(() => start.from
    ? start.from.steps.map((s) => ({ ...s }))
    : [{ name: "", target: devices[0]?.id ?? "", command: "" }]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const setStep = (index: number, patch: Partial<StepDraft>) =>
    setSteps((list) => list.map((step, i) => (i === index ? { ...step, ...patch } : step)));

  const saveCommand = async () => {
    const found: Record<string, string> = {};
    if (!name.trim()) found.name = t("nameRequired");
    steps.forEach((step, i) => {
      if (!step.target) found[`dev${i}`] = t("deviceRequired");
      else if (!known.has(step.target)) found[`dev${i}`] = t("deviceGone");
      if (!step.command.trim()) found[`cmd${i}`] = t("commandRequired");
    });
    setErrors(found);
    if (Object.keys(found).length) return;
    const request: CreateSavedActionRequest = {
      name: name.trim(),
      ...(description.trim() ? { description: description.trim() } : {}),
      steps: steps.map((step, i) => ({
        name: step.name.trim() || t("defaultStepName", { number: i + 1 }),
        command: step.command.trim(),
        target: step.target,
      })),
    };
    setPending(true);
    setFormError(null);
    try {
      const created = await createAction(request);
      addToast("success", t("created", { name: created.name }));
      onClose(created);
    } catch (cause) {
      setFormError(errorText(cause, t("createFailed")));
    } finally {
      setPending(false);
    }
  };

  const runIntegration = async () => {
    const found = integrationErrors(integration, ti);
    setErrors(found);
    if (Object.keys(found).length || !integration.selectedActionDescriptor) return;
    setFormError(null);
    const outcome = await integration.submitConnectorAction();
    const actionName = integration.selectedActionDescriptor.name;
    if (!outcome.ok) {
      setFormError(errorText(new Error(outcome.error), ti("failed")));
      return;
    }
    addToast("success", outcome.dryRun ? ti("previewQueued", { name: actionName }) : ti("queued", { name: actionName }));
    if (!outcome.dryRun) onClose();
  };

  const preview = integration.actionSupportsDryRun && integration.actionDryRun;
  const isCommand = kind === "command";

  return (
    <EditorSheet
      title={start.from ? t("copyTitle") : t("newTitle")}
      submitLabel={isCommand ? tc("create") : preview ? ti("runPreview") : ti("run")}
      cancelLabel={tc("cancel")}
      onSubmit={() => { void (isCommand ? saveCommand() : runIntegration()); }}
      onCancel={() => onClose()}
      pending={pending || integration.actionSubmitting}
      error={formError}
      footStart={isCommand ? null : ti("once")}
    >
      {start.from ? null : (
        <SegmentedTabs
          className="au-kind"
          value={kind}
          options={(["command", "integration"] as const).map((k) => ({ id: k, label: t(k) }))}
          onChange={(k) => { setKind(k); setErrors({}); setFormError(null); }}
        />
      )}
      {isCommand ? (
        <>
          <div className="au-grid">
            <Field id="af-name" label={t("name")} error={errors.name}>
              <Input id="af-name" value={name} maxLength={200} autoComplete="off" onChange={(e) => setName(e.target.value)} {...fieldAria("af-name", errors.name)} />
            </Field>
            <Field id="af-desc" label={t("description")}>
              <Input id="af-desc" value={description} maxLength={1000} autoComplete="off" placeholder={t("optional")} onChange={(e) => setDescription(e.target.value)} />
            </Field>
          </div>
          {devices.length === 0 ? <p className="au-hint au-gap" role="status">{t("noDevices")}</p> : null}
          <div className="au-steps" role="group" aria-label={t("steps")}>
            <div className="au-step au-step-h" aria-hidden="true"><span /><span>{t("device")}</span><span>{t("command")}</span><span /></div>
            {steps.map((step, i) => (
              <div className="au-step" key={i}>
                <span className="au-sn num" aria-hidden="true">{i + 1}</span>
                <div className="au-sc">
                  <Select className="w-full" id={`af-s${i}-dev`} value={step.target} aria-label={t("stepDevice", { number: i + 1 })} onChange={(e) => setStep(i, { target: e.target.value })} {...fieldAria(`af-s${i}-dev`, errors[`dev${i}`])}>
                    {!step.target ? <option value="" disabled>{t("device")}</option> : null}
                    {step.target && !known.has(step.target) ? <option value={step.target}>{step.target}</option> : null}
                    {devices.map((d) => <option key={d.id} value={d.id}>{d.name || d.id}</option>)}
                  </Select>
                  {errors[`dev${i}`] ? <p className="au-err" id={`af-s${i}-dev-err`}>{errors[`dev${i}`]}</p> : null}
                </div>
                <div className="au-sc">
                  <Textarea
                    id={`af-s${i}-cmd`}
                    className="au-mono au-line"
                    rows={1}
                    value={step.command}
                    maxLength={4096}
                    spellCheck={false}
                    autoComplete="off"
                    placeholder="docker restart jellyfin"
                    aria-label={t("stepCommand", { number: i + 1 })}
                    onChange={(e) => setStep(i, { command: e.target.value })}
                    {...fieldAria(`af-s${i}-cmd`, errors[`cmd${i}`])}
                  />
                  {errors[`cmd${i}`] ? <p className="au-err" id={`af-s${i}-cmd-err`}>{errors[`cmd${i}`]}</p> : null}
                </div>
                {steps.length > 1 ? (
                  <button type="button" className="au-ib" aria-label={t("removeStep", { number: i + 1 })} onClick={() => setSteps((list) => list.filter((_, j) => j !== i))}>
                    <X aria-hidden="true" />
                  </button>
                ) : <span />}
              </div>
            ))}
          </div>
          {steps.length < MAX_STEPS ? (
            <button type="button" className="au-add" onClick={() => setSteps((list) => [...list, { name: "", target: list.at(-1)?.target ?? devices[0]?.id ?? "", command: "" }])}>
              <Plus aria-hidden="true" />{t("addStep")}
            </button>
          ) : null}
        </>
      ) : (
        <IntegrationFields integration={integration} assets={devices} errors={errors} />
      )}
    </EditorSheet>
  );
}
