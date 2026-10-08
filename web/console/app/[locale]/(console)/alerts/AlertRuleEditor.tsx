"use client";

import { type Dispatch, type SetStateAction, type FormEvent } from "react";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import type { AlertRuleTemplate, Asset, Group } from "../../../console/models";
import type { RuleKind, RuleSeverity, TargetType } from "./alertsPageTypes";
import { ruleKindOptions, ruleSeverityOptions, targetTypeOptions } from "./alertsPageTypes";
import { type RuleFormState } from "./alertRuleFormState";

export type AlertRuleEditorProps = {
  ruleForm: RuleFormState;
  setRuleForm: Dispatch<SetStateAction<RuleFormState>>;
  setField: <K extends keyof RuleFormState>(field: K, value: RuleFormState[K]) => void;
  selectedPreset: AlertRuleTemplate | null;
  targetOptions: Asset[] | Group[];
  assets: Asset[];
  groups: Group[];
  groupLabelByID: Map<string, string>;
  ruleError: string | null;
  ruleSubmitting: boolean;
  handleCreateRule: (event: FormEvent) => Promise<void>;
  resetRuleForm: () => void;
  setShowRuleForm: (show: boolean) => void;
};

export function AlertRuleEditor({
  ruleForm,
  setRuleForm,
  setField,
  selectedPreset,
  targetOptions,
  assets,
  groups,
  groupLabelByID,
  ruleError,
  ruleSubmitting,
  handleCreateRule,
  resetRuleForm,
  setShowRuleForm,
}: AlertRuleEditorProps) {
  return (
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
  );
}
