"use client";

import { useMemo, useState, type FormEvent } from "react";
import { Pencil, Sparkles } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Input, Select } from "../../../components/ui/Input";
import { formatMetadataLabel } from "../../../console/formatters";
import type { AlertRule, AlertRuleTemplate, Asset, Group } from "../../../console/models";
import { useFastStatus, useGroupLabelByID, useSlowStatus } from "../../../contexts/StatusContext";
import type {
  RuleKind,
  RuleSeverity,
  TargetType,
} from "./alertsPageTypes";
import { ruleKindOptions, ruleSeverityOptions, targetTypeOptions } from "./alertsPageTypes";

type AlertRulesTabProps = {
  rules: AlertRule[];
  templates: AlertRuleTemplate[];
  highlightedRuleId: string | null;
  onHighlightedRuleIdChange: (ruleId: string | null) => void;
  createRule: (rule: Record<string, unknown>) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
};

type RuleFormState = {
  name: string;
  description: string;
  kind: RuleKind;
  severity: RuleSeverity;
  targetType: TargetType;
  targetId: string;
  windowSeconds: number;
  cooldownSeconds: number;
  reopenAfterSeconds: number;
  evaluationIntervalSeconds: number;
  metric: string;
  operator: string;
  thresholdValue: number;
  aggregate: string;
  maxSilenceSeconds: number;
  maxStaleSeconds: number;
  pattern: string;
  minOccurrences: number;
  checkId: string;
  consecutiveFailures: number;
  subRuleIds: string;
  compositeOperator: "and" | "or";
};

const defaultRuleFormState: RuleFormState = {
  name: "",
  description: "",
  kind: "metric_threshold",
  severity: "high",
  targetType: "global",
  targetId: "",
  windowSeconds: 300,
  cooldownSeconds: 300,
  reopenAfterSeconds: 120,
  evaluationIntervalSeconds: 30,
  metric: "",
  operator: ">",
  thresholdValue: 90,
  aggregate: "avg",
  maxSilenceSeconds: 300,
  maxStaleSeconds: 300,
  pattern: "",
  minOccurrences: 5,
  checkId: "",
  consecutiveFailures: 3,
  subRuleIds: "",
  compositeOperator: "and",
};

function createRuleFormState(overrides: Partial<RuleFormState> = {}): RuleFormState {
  return {
    ...defaultRuleFormState,
    ...overrides,
  };
}

function numberFromUnknown(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function stringFromUnknown(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function formStateFromTemplate(template: AlertRuleTemplate): RuleFormState {
  const condition = template.condition ?? {};
  return createRuleFormState({
    name: template.name,
    description: template.description,
    kind: template.kind,
    severity: template.severity,
    targetType: template.target_scope,
    windowSeconds: template.window_seconds,
    cooldownSeconds: template.cooldown_seconds,
    reopenAfterSeconds: template.reopen_after_seconds,
    evaluationIntervalSeconds: template.evaluation_interval_seconds,
    metric: stringFromUnknown(condition.metric),
    operator: stringFromUnknown(condition.operator, ">"),
    thresholdValue: numberFromUnknown(condition.value, 0),
    aggregate: stringFromUnknown(condition.aggregate, "avg"),
    maxSilenceSeconds: numberFromUnknown(condition.max_silence_seconds, defaultRuleFormState.maxSilenceSeconds),
    maxStaleSeconds: numberFromUnknown(condition.max_stale_seconds, defaultRuleFormState.maxStaleSeconds),
    pattern: stringFromUnknown(condition.pattern),
    minOccurrences: numberFromUnknown(condition.min_occurrences, defaultRuleFormState.minOccurrences),
    checkId: stringFromUnknown(condition.check_id),
    consecutiveFailures: numberFromUnknown(condition.consecutive_failures, defaultRuleFormState.consecutiveFailures),
    subRuleIds: Array.isArray(condition.rule_ids)
      ? condition.rule_ids.map((value) => String(value)).join(", ")
      : "",
    compositeOperator: stringFromUnknown(condition.operator) === "or" ? "or" : "and",
  });
}

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
      setRuleError(`Choose a ${ruleForm.targetType} target for this rule.`);
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
        <form className="space-y-4 border-t border-[var(--line)] py-4" onSubmit={(event) => void handleCreateRule(event)}>
          {selectedPreset ? (
            <div className="rounded-2xl border border-[var(--line)] bg-[var(--panel)] px-3 py-2 text-xs text-[var(--muted)]">
              Building from template: <span className="font-medium text-[var(--text)]">{selectedPreset.name}</span>
            </div>
          ) : null}

          <div className="grid gap-3 md:grid-cols-2">
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Name
              <Input
                value={ruleForm.name}
                onChange={(event) => setField("name", event.target.value)}
                placeholder="e.g. High CPU Alert"
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Description
              <Input
                value={ruleForm.description}
                onChange={(event) => setField("description", event.target.value)}
                placeholder="Optional operator context"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Kind
              <Select value={ruleForm.kind} onChange={(event) => setField("kind", event.target.value as RuleKind)}>
                {ruleKindOptions.map((option) => (
                  <option key={option.id} value={option.id}>{option.label}</option>
                ))}
              </Select>
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Severity
              <Select value={ruleForm.severity} onChange={(event) => setField("severity", event.target.value as RuleSeverity)}>
                {ruleSeverityOptions.map((option) => (
                  <option key={option.id} value={option.id}>{option.label}</option>
                ))}
              </Select>
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Target Type
              <Select value={ruleForm.targetType} onChange={(event) => {
                const nextType = event.target.value as TargetType;
                setRuleForm((current) => ({
                  ...current,
                  targetType: nextType,
                  targetId: "",
                }));
              }}>
                {targetTypeOptions.map((option) => (
                  <option key={option.id} value={option.id}>{option.label}</option>
                ))}
              </Select>
            </label>
            {ruleForm.targetType !== "global" ? (
              <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                {ruleForm.targetType === "asset" ? "Target Asset" : "Target Group"}
                {targetOptions.length > 0 ? (
                  <Select value={ruleForm.targetId} onChange={(event) => setField("targetId", event.target.value)}>
                    <option value="">{ruleForm.targetType === "asset" ? "Select an asset" : "Select a group"}</option>
                    {ruleForm.targetType === "asset"
                      ? assets.map((asset) => (
                        <option key={asset.id} value={asset.id}>
                          {asset.name}{asset.group_id ? ` - ${groupLabelByID.get(asset.group_id) ?? asset.group_id}` : ""}
                        </option>
                      ))
                      : groups.map((group) => (
                        <option key={group.id} value={group.id}>
                          {group.name}
                        </option>
                      ))}
                  </Select>
                ) : (
                  <Input
                    value={ruleForm.targetId}
                    onChange={(event) => setField("targetId", event.target.value)}
                    placeholder={ruleForm.targetType === "asset" ? "Asset ID" : "Group ID"}
                  />
                )}
              </label>
            ) : null}
          </div>

          <div className="grid gap-3 md:grid-cols-4">
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Eval Window (seconds)
              <Input
                type="number"
                min={1}
                value={ruleForm.windowSeconds}
                onChange={(event) => setField("windowSeconds", Number(event.target.value))}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Eval Interval (seconds)
              <Input
                type="number"
                min={1}
                value={ruleForm.evaluationIntervalSeconds}
                onChange={(event) => setField("evaluationIntervalSeconds", Number(event.target.value))}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Cooldown (seconds)
              <Input
                type="number"
                min={0}
                value={ruleForm.cooldownSeconds}
                onChange={(event) => setField("cooldownSeconds", Number(event.target.value))}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Reopen After (seconds)
              <Input
                type="number"
                min={0}
                value={ruleForm.reopenAfterSeconds}
                onChange={(event) => setField("reopenAfterSeconds", Number(event.target.value))}
              />
            </label>
          </div>

          <div className="space-y-3 rounded-2xl border border-[var(--line)] bg-[var(--panel)] p-4">
            {ruleForm.kind === "metric_threshold" ? (
              <div className="grid gap-3 md:grid-cols-4">
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)] md:col-span-2">
                  Metric
                  <Input value={ruleForm.metric} onChange={(event) => setField("metric", event.target.value)} placeholder="e.g. cpu_used_percent" />
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Operator
                  <Select value={ruleForm.operator} onChange={(event) => setField("operator", event.target.value)}>
                    <option value=">">&gt;</option>
                    <option value="<">&lt;</option>
                    <option value=">=">&gt;=</option>
                    <option value="<=">&lt;=</option>
                    <option value="==">==</option>
                  </Select>
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Threshold
                  <Input type="number" value={ruleForm.thresholdValue} onChange={(event) => setField("thresholdValue", Number(event.target.value))} />
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Aggregate
                  <Select value={ruleForm.aggregate} onChange={(event) => setField("aggregate", event.target.value)}>
                    <option value="avg">avg</option>
                    <option value="max">max</option>
                    <option value="min">min</option>
                    <option value="last">last</option>
                  </Select>
                </label>
              </div>
            ) : null}

            {ruleForm.kind === "metric_deadman" ? (
              <div className="grid gap-3 md:grid-cols-2">
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Metric
                  <Input value={ruleForm.metric} onChange={(event) => setField("metric", event.target.value)} placeholder="e.g. cpu_used_percent" />
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Max Silence (seconds)
                  <Input type="number" min={1} value={ruleForm.maxSilenceSeconds} onChange={(event) => setField("maxSilenceSeconds", Number(event.target.value))} />
                </label>
              </div>
            ) : null}

            {ruleForm.kind === "heartbeat_stale" ? (
              <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                Max Stale (seconds)
                <Input type="number" min={1} value={ruleForm.maxStaleSeconds} onChange={(event) => setField("maxStaleSeconds", Number(event.target.value))} />
              </label>
            ) : null}

            {ruleForm.kind === "log_pattern" ? (
              <div className="grid gap-3 md:grid-cols-2">
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Pattern
                  <Input value={ruleForm.pattern} onChange={(event) => setField("pattern", event.target.value)} placeholder="e.g. ERROR|FATAL|panic" />
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Min Occurrences
                  <Input type="number" min={1} value={ruleForm.minOccurrences} onChange={(event) => setField("minOccurrences", Number(event.target.value))} />
                </label>
              </div>
            ) : null}

            {ruleForm.kind === "synthetic_check" ? (
              <div className="grid gap-3 md:grid-cols-2">
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Check ID
                  <Input value={ruleForm.checkId} onChange={(event) => setField("checkId", event.target.value)} placeholder="Synthetic check ID" />
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Consecutive Failures
                  <Input type="number" min={1} value={ruleForm.consecutiveFailures} onChange={(event) => setField("consecutiveFailures", Number(event.target.value))} />
                </label>
              </div>
            ) : null}

            {ruleForm.kind === "composite" ? (
              <div className="grid gap-3 md:grid-cols-2">
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Sub-Rule IDs
                  <Input value={ruleForm.subRuleIds} onChange={(event) => setField("subRuleIds", event.target.value)} placeholder="rule-id-1, rule-id-2" />
                </label>
                <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                  Combine With
                  <Select value={ruleForm.compositeOperator} onChange={(event) => setField("compositeOperator", event.target.value as "and" | "or")}>
                    <option value="and">All rules firing</option>
                    <option value="or">Any rule firing</option>
                  </Select>
                </label>
              </div>
            ) : null}
          </div>

          {ruleError ? <p className="text-xs text-[var(--bad)]">{ruleError}</p> : null}
          <div className="flex items-center gap-3 pt-2">
            <Button type="submit" variant="primary" disabled={ruleSubmitting}>
              {ruleSubmitting ? "Creating..." : "Create Rule"}
            </Button>
            <Button
              type="button"
              onClick={() => {
                resetRuleForm();
                setShowRuleForm(false);
              }}
            >
              Cancel
            </Button>
          </div>
        </form>
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
