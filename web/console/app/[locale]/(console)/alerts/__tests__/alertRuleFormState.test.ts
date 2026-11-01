import { describe, expect, it } from "vitest";
import type { AlertRule } from "../../../../console/models";
import { createRuleFormState, formStateFromRule, ruleFormErrors, rulePayload, ruleUpdatePayload } from "../alertRuleFormState";

// Risk: the editor is the only way to add a rule. A form that passes the check must post the body the hub expects
// for its kind, target and labels (silences match them); a form that would fail at the hub must be stopped with the
// field that is wrong.
describe("rule editor values", () => {
  it("builds a metric threshold rule for one device", () => {
    const form = createRuleFormState({ name: " CPU above 90% ", metric: "cpu_used_percent", targetType: "asset", targetId: "nas01",
      labels: [{ key: " team ", value: "lab" }, { key: "", value: "dropped" }] });
    expect(ruleFormErrors(form, [])).toEqual({});
    expect(rulePayload(form)).toMatchObject({
      name: "CPU above 90%",
      kind: "metric_threshold",
      status: "active",
      target_scope: "asset",
      targets: [{ asset_id: "nas01" }],
      condition: { metric: "cpu_used_percent", operator: ">", value: 90, aggregate: "avg" },
      labels: { team: "lab" },
    });
  });

  it("names the fields that stop a save", () => {
    const form = createRuleFormState({ name: "Disk full", kind: "log_pattern", targetType: "group", windowSeconds: NaN, minOccurrences: 0 });
    expect(ruleFormErrors(form, ["disk FULL"])).toEqual({ name: "nameTaken", target: "target", window: "whole", pattern: "pattern", count: "whole" });
    expect(ruleFormErrors(createRuleFormState({ name: "Both", kind: "composite" }), [])).toEqual({ rules: "rules" });
  });

  it("posts a combined rule with the chosen rule IDs and no targets when global", () => {
    const payload = rulePayload(createRuleFormState({ name: "Both", kind: "composite", subRuleIds: ["r1", "r2"], compositeOperator: "or" }));
    expect(payload.condition).toEqual({ rule_ids: ["r1", "r2"], operator: "or" });
    expect(payload).not.toHaveProperty("targets");
  });

  it("opens an existing rule's settings and first target in the form", () => {
    const rule = {
      id: "r1", name: "Camera stops checking in", status: "active", kind: "heartbeat_stale", severity: "critical", target_scope: "asset",
      cooldown_seconds: 600, window_seconds: 120, condition: { max_stale_seconds: 900 }, targets: [{ id: "t1", asset_id: "rpi-camera" }],
      created_at: "", updated_at: "",
    } as AlertRule;
    expect(formStateFromRule(rule)).toMatchObject({ kind: "heartbeat_stale", severity: "critical", targetType: "asset", targetId: "rpi-camera", maxStaleSeconds: 900, cooldownSeconds: 600 });
  });
});

// Risk: the hub replaces a rule's condition and targets whole. A change must send only what the person changed, so
// an edit never drops condition keys the form does not show, extra targets, or the rule's on/off state.
describe("rule change body", () => {
  const rule = {
    id: "r2", name: "Error flood", status: "paused", kind: "log_pattern", severity: "high", target_scope: "asset",
    cooldown_seconds: 300, window_seconds: 300, condition: { pattern: "error", min_occurrences: 5, source: "syslog" },
    labels: { team: "lab" }, targets: [{ id: "t1", asset_id: "nas01" }, { id: "t2", asset_id: "nas02" }], created_at: "", updated_at: "",
  } as AlertRule;
  const before = formStateFromRule(rule);

  it("sends nothing the form left alone", () => {
    expect(ruleUpdatePayload(rule, before, { ...before })).toEqual({});
  });

  it("sends the changed fields and keeps the condition keys the form does not show", () => {
    const after = { ...before, severity: "critical" as const, minOccurrences: 10, labels: [] };
    expect(ruleUpdatePayload(rule, before, after)).toEqual({
      severity: "critical",
      condition: { pattern: "error", min_occurrences: 10, source: "syslog" },
      labels: {},
    });
  });

  it("drops the old threshold name so the hub does not refuse a new value", () => {
    const metric = { ...rule, kind: "metric_threshold", condition: { metric: "cpu_used_percent", operator: ">", threshold: 80 } } as AlertRule;
    const start = formStateFromRule(metric);
    expect(ruleUpdatePayload(metric, start, { ...start, thresholdValue: 95 }).condition)
      .toEqual({ metric: "cpu_used_percent", operator: ">", value: 95, aggregate: "avg" });
  });
});
