"use client";

import { useMemo, useState, type FormEvent } from "react";
import { Pencil, Sparkles } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { formatMetadataLabel } from "../../../console/formatters";
import type { AlertRule, AlertRuleTemplate, Asset, Group } from "../../../console/models";
import { useFastStatus, useGroupLabelByID, useSlowStatus } from "../../../contexts/StatusContext";
import { type RuleFormState, createRuleFormState, formStateFromTemplate } from "./alertRuleFormState";
import { AlertRuleEditor } from "./AlertRuleEditor";

type AlertRulesTabProps = {
  rules: AlertRule[];
  templates: AlertRuleTemplate[];
  highlightedRuleId: string | null;
  onHighlightedRuleIdChange: (ruleId: string | null) => void;
  createRule: (rule: Record<string, unknown>) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
};

function describeRuleTargets(rule: AlertRule, assetsByID: Map<string, Asset>, groupsByID: Map<string, Group>): string {
  if (rule.target_scope === "global") {
    return "Global";
  }
  if (!Array.isArray(rule.targets) || rule.targets.length === 0) {
    return formatMetadataLabel(rule.target_scope);
  }

  const labels = rule.targets.map((target) => {
    if (target.asset_id) {
      const asset = assetsByID.get(target.asset_id);
      return asset?.name ?? target.asset_id;
    }
    if (target.group_id) {
      const group = groupsByID.get(target.group_id);
      return group?.name ?? target.group_id;
    }
    return target.id;
  });

  return labels.join(", ");
}

export function AlertRulesTab({
  rules,
  templates,
  highlightedRuleId,
  onHighlightedRuleIdChange,
  createRule,
  deleteRule,
}: AlertRulesTabProps) {
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const groupLabelByID = useGroupLabelByID();
  const [showRuleForm, setShowRuleForm] = useState(false);
  const [ruleForm, setRuleForm] = useState<RuleFormState>(createRuleFormState());
  const [ruleSubmitting, setRuleSubmitting] = useState(false);
  const [ruleError, setRuleError] = useState<string | null>(null);
  const [presetId, setPresetId] = useState<string | null>(null);

  const assets = useMemo(() => (
    [...(status?.assets ?? [])].sort((left, right) => left.name.localeCompare(right.name))
  ), [status?.assets]);
  const groups = useMemo(() => (
    [...(slowStatus?.groups ?? [])].sort((left, right) => left.name.localeCompare(right.name))
  ), [slowStatus?.groups]);

  const assetsByID = useMemo(() => new Map(assets.map((asset) => [asset.id, asset])), [assets]);
  const groupsByID = useMemo(() => new Map(groups.map((group) => [group.id, group])), [groups]);
  const sortedTemplates = useMemo(() => {
    return [...templates].sort((left, right) => {
      const leftStarter = left.metadata?.category === "starter" ? 0 : 1;
      const rightStarter = right.metadata?.category === "starter" ? 0 : 1;
      if (leftStarter !== rightStarter) {
        return leftStarter - rightStarter;
      }
      return left.name.localeCompare(right.name);
    });
  }, [templates]);
  const selectedPreset = presetId ? sortedTemplates.find((template) => template.id === presetId) ?? null : null;
  const targetOptions = ruleForm.targetType === "asset" ? assets : groups;

  function setField<K extends keyof RuleFormState>(field: K, value: RuleFormState[K]) {
    setRuleForm((current) => ({ ...current, [field]: value }));
  }

  function resetRuleForm() {
    setRuleForm(createRuleFormState());
    setRuleError(null);
    setRuleSubmitting(false);
    setPresetId(null);
  }

  function openBlankForm() {
    setShowRuleForm(true);
    resetRuleForm();
  }

  function applyPreset(template: AlertRuleTemplate) {
    setShowRuleForm(true);
    setRuleError(null);
    setRuleSubmitting(false);
    setPresetId(template.id);
    setRuleForm(formStateFromTemplate(template));
  }

  function validateAndBuildCondition(): Record<string, unknown> | null {
    switch (ruleForm.kind) {
      case "metric_threshold":
        if (!ruleForm.metric.trim()) {
          setRuleError("Metric name is required for metric threshold rules.");
          return null;
        }
        return {
          metric: ruleForm.metric.trim(),
          operator: ruleForm.operator,
          value: ruleForm.thresholdValue,
          aggregate: ruleForm.aggregate,
        };
      case "metric_deadman":
        if (!ruleForm.metric.trim()) {
          setRuleError("Metric name is required for deadman rules.");
          return null;
        }
        return {
          metric: ruleForm.metric.trim(),
          max_silence_seconds: ruleForm.maxSilenceSeconds,
        };
      case "heartbeat_stale":
        return { max_stale_seconds: ruleForm.maxStaleSeconds };
      case "log_pattern":
        if (!ruleForm.pattern.trim()) {
          setRuleError("Pattern is required for log pattern rules.");
          return null;
        }
        return {
          pattern: ruleForm.pattern.trim(),
          min_occurrences: ruleForm.minOccurrences,
        };
      case "synthetic_check":
        if (!ruleForm.checkId.trim()) {
          setRuleError("Check ID is required for synthetic check rules.");
          return null;
        }
        return {
          check_id: ruleForm.checkId.trim(),
          consecutive_failures: ruleForm.consecutiveFailures,
        };
      case "composite": {
        const ruleIDs = ruleForm.subRuleIds.split(",").map((value) => value.trim()).filter(Boolean);
        if (ruleIDs.length === 0) {
          setRuleError("Add at least one sub-rule ID for composite rules.");
          return null;
        }
        return {
          rule_ids: ruleIDs,
          operator: ruleForm.compositeOperator,
        };
      }
    }
  }

  async function handleCreateRule(event: FormEvent) {
    event.preventDefault();
    setRuleError(null);

    if (!ruleForm.name.trim()) {
      setRuleError("Rule name is required.");
      return;
    }
    if (ruleForm.targetType !== "global" && !ruleForm.targetId.trim()) {
      const article = ruleForm.targetType === "asset" ? "an" : "a";
      setRuleError(`Choose ${article} ${ruleForm.targetType} target for this rule.`);
      return;
    }

    const condition = validateAndBuildCondition();
    if (!condition) {
      return;
    }

    const payload: Record<string, unknown> = {
      name: ruleForm.name.trim(),
      description: ruleForm.description.trim(),
      kind: ruleForm.kind,
      severity: ruleForm.severity,
      status: "active",
      target_scope: ruleForm.targetType,
      window_seconds: ruleForm.windowSeconds,
      cooldown_seconds: ruleForm.cooldownSeconds,
      reopen_after_seconds: ruleForm.reopenAfterSeconds,
      evaluation_interval_seconds: ruleForm.evaluationIntervalSeconds,
      condition,
    };

    if (ruleForm.targetType === "asset") {
      payload.targets = [{ asset_id: ruleForm.targetId.trim() }];
    } else if (ruleForm.targetType === "group") {
      payload.targets = [{ group_id: ruleForm.targetId.trim() }];
    }

    setRuleSubmitting(true);
    try {
      await createRule(payload);
      resetRuleForm();
      setShowRuleForm(false);
    } catch (err) {
      setRuleError(err instanceof Error ? err.message : "Could not create rule");
    } finally {
      setRuleSubmitting(false);
    }
  }

  async function handleDeleteRule(id: string) {
    try {
      await deleteRule(id);
    } catch {
      // Preserve the existing quiet failure behavior in this list action.
    }
  }

  return (
    <Card className="mb-4">
      <div className="flex flex-col gap-3 mb-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2>Alert Rules</h2>
            <p className="text-sm text-[var(--muted)]">
              Start with a shared template or build a custom rule against metrics, heartbeats, logs, checks, or other rules.
            </p>
          </div>
          <Button size="sm" onClick={() => {
            if (showRuleForm) {
              resetRuleForm();
              setShowRuleForm(false);
            } else {
              openBlankForm();
            }
          }}>
            {showRuleForm ? "Cancel" : "New Rule"}
          </Button>
        </div>

        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {sortedTemplates.map((template) => (
            <div key={template.id} className="rounded-2xl border border-[var(--line)] bg-[var(--panel)] p-4">
              <div className="mb-3 flex items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-2">
                    <Sparkles className="h-4 w-4 text-[var(--accent)]" />
                    <h3 className="text-sm font-semibold text-[var(--text)]">{template.name}</h3>
                  </div>
                  <p className="mt-1 text-xs leading-5 text-[var(--muted)]">{template.description}</p>
                </div>
              </div>
              <div className="mb-3 flex flex-wrap items-center gap-2">
                <Badge status={template.severity} />
                <span className="rounded-full border border-[var(--line)] px-2 py-1 text-xs text-[var(--muted)]">
                  {formatMetadataLabel(template.kind)}
                </span>
                <span className="rounded-full border border-[var(--line)] px-2 py-1 text-xs text-[var(--muted)]">
                  {formatMetadataLabel(template.target_scope)}
                </span>
                {template.metadata?.category === "starter" ? (
                  <span className="rounded-full border border-[var(--line)] px-2 py-1 text-xs text-[var(--muted)]">
                    Starter
                  </span>
                ) : null}
              </div>
              <Button size="sm" variant="primary" onClick={() => applyPreset(template)}>
                Use Template
              </Button>
            </div>
          ))}
        </div>
      </div>

      {showRuleForm ? (
        <AlertRuleEditor
          ruleForm={ruleForm}
          setRuleForm={setRuleForm}
          setField={setField}
          selectedPreset={selectedPreset}
          targetOptions={targetOptions}
          assets={assets}
          groups={groups}
          groupLabelByID={groupLabelByID}
          ruleError={ruleError}
          ruleSubmitting={ruleSubmitting}
          handleCreateRule={handleCreateRule}
          resetRuleForm={resetRuleForm}
          setShowRuleForm={setShowRuleForm}
        />
      ) : null}

      {rules.length === 0 && !showRuleForm ? (
        <EmptyState
          icon={Pencil}
          title="No alert rules yet"
          description="Start with a starter rule or create a custom rule that watches your devices and services."
        />
      ) : rules.length > 0 ? (
        <ul className="divide-y divide-[var(--line)] border-t border-[var(--line)] pt-2">
          {rules.map((rule) => (
            <li
              key={rule.id}
              className={`flex items-center justify-between gap-3 py-2.5${highlightedRuleId === rule.id ? " bg-[var(--hover)]" : ""}`}
              onClick={() => onHighlightedRuleIdChange(highlightedRuleId === rule.id ? null : rule.id)}
            >
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-[var(--text)]">{rule.name}</span>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--muted)]">
                  <code>{formatMetadataLabel(rule.kind)}</code>
                  <span>&middot;</span>
                  <span>{describeRuleTargets(rule, assetsByID, groupsByID)}</span>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Badge status={rule.severity} />
                <Badge status={rule.status} />
                <Button
                  variant="danger"
                  size="sm"
                  onClick={(event) => {
                    event.stopPropagation();
                    void handleDeleteRule(rule.id);
                  }}
                  title="Delete rule"
                >
                  Delete
                </Button>
              </div>
            </li>
          ))}
        </ul>
      ) : null}
    </Card>
  );
}
