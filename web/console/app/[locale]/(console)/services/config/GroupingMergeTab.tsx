"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { RuntimeSettingEntry, RuntimeSettingsPayload } from "../../../../console/models";
import { runtimeSettingKeys } from "../../../../console/models";

// ---------------------------------------------------------------------------
// Shared types & helpers
// ---------------------------------------------------------------------------

type GroupingMode = "off" | "conservative" | "balanced" | "aggressive";

function normalizeMode(value: string): GroupingMode {
  const lowered = value.trim().toLowerCase();
  if (
    lowered === "off" ||
    lowered === "conservative" ||
    lowered === "balanced" ||
    lowered === "aggressive"
  ) {
    return lowered;
  }
  return "balanced";
}

function normalizeThreshold(value: string, fallback: string): string {
  const parsed = Number.parseInt(value.trim(), 10);
  if (!Number.isFinite(parsed)) return fallback;
  return String(Math.max(0, Math.min(100, parsed)));
}

function parseBool(value: string, fallback: boolean): boolean {
  const lowered = value.trim().toLowerCase();
  if (lowered === "true") return true;
  if (lowered === "false") return false;
  return fallback;
}

function appendRuleLine(existing: string, nextRule: string): string {
  const normalizedRule = nextRule.trim();
  if (!normalizedRule) return existing;
  const lines = existing
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
  const seen = new Set(lines.map((line) => line.toLowerCase()));
  if (!seen.has(normalizedRule.toLowerCase())) {
    lines.push(normalizedRule);
  }
  return lines.join("\n");
}

function normalizeBaseDomain(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^\*?\./, "")
    .replace(/\.+$/, "");
}

// ---------------------------------------------------------------------------
// Source badge helpers (merge card only)
// ---------------------------------------------------------------------------

function sourceLabel(source: RuntimeSettingEntry["source"] | undefined): string {
  switch (source) {
    case "ui":
      return "UI";
    case "docker":
      return "Docker";
    default:
      return "Default";
  }
}

function sourceClassName(source: RuntimeSettingEntry["source"] | undefined): string {
  switch (source) {
    case "ui":
      return "bg-[var(--ok)]/15 text-[var(--ok)]";
    case "docker":
      return "bg-[var(--warn)]/15 text-[var(--warn)]";
    default:
      return "bg-[var(--surface)] text-[var(--muted)]";
  }
}

// ---------------------------------------------------------------------------
// Mode option descriptors
// ---------------------------------------------------------------------------

const groupingModeValues: GroupingMode[] = ["off", "conservative", "balanced", "aggressive"];

const mergeModeValues: GroupingMode[] = ["off", "conservative", "balanced", "aggressive"];

// ---------------------------------------------------------------------------
// Runtime setting keys managed by the merge card
// ---------------------------------------------------------------------------

const managedMergeKeys = [
  runtimeSettingKeys.servicesMergeMode,
  runtimeSettingKeys.servicesMergeConfidenceThreshold,
  runtimeSettingKeys.servicesMergeDryRun,
  runtimeSettingKeys.servicesMergeAliasRules,
  runtimeSettingKeys.servicesForceMergeRules,
  runtimeSettingKeys.servicesNeverMergeRules,
] as const;

// =========================================================================
// URL Grouping Card
// =========================================================================

type GroupingDraft = {
  mode: GroupingMode;
  confidenceThreshold: string;
  aliasRules: string;
  neverGroupRules: string;
};

const defaultGroupingDraft: GroupingDraft = {
  mode: "balanced",
  confidenceThreshold: "85",
  aliasRules: "",
  neverGroupRules: "",
};

function URLGroupingCard() {
  const t = useTranslations("services");

  const [expanded, setExpanded] = useState(true);
  const [draft, setDraft] = useState<GroupingDraft>(defaultGroupingDraft);
  const [presetDomain, setPresetDomain] = useState("simbaslabs.com");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const loadSettings = useCallback(async () => {
    setLoading(true);
    setError(null);
    setMessage(null);
    try {
      const res = await fetch("/api/services/web/grouping-settings", {
        cache: "no-store",
      });
      if (!res.ok) {
        throw new Error(`Failed to load settings (HTTP ${res.status})`);
      }
      const data = await res.json();
      const settingsMap: Record<string, string> = {};
      for (const s of data.settings ?? []) {
        settingsMap[s.setting_key] = s.setting_value;
      }
      setDraft({
        mode: normalizeMode(settingsMap["grouping_mode"] || "balanced"),
        confidenceThreshold: settingsMap["sensitivity"] || "85",
        aliasRules: settingsMap["alias_rules"] || "",
        neverGroupRules: settingsMap["never_group_rules"] || "",
      });
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to load URL grouping settings"
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  const save = useCallback(async () => {
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const res = await fetch("/api/services/web/grouping-settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          values: {
            grouping_mode: draft.mode,
            sensitivity: String(
              normalizeThreshold(draft.confidenceThreshold, "85")
            ),
            alias_rules: draft.aliasRules.trim(),
            never_group_rules: draft.neverGroupRules.trim(),
          },
        }),
      });
      if (!res.ok) {
        throw new Error(`Failed to save settings (HTTP ${res.status})`);
      }
      setMessage(t("config.grouping.saved"));
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to save URL grouping settings"
      );
    } finally {
      setSaving(false);
    }
  }, [draft, t]);

  const previewDomain = normalizeBaseDomain(presetDomain) || "simbaslabs.com";

  const addMiddleLabelPreset = useCallback(() => {
    const domain = normalizeBaseDomain(presetDomain);
    if (!domain) {
      setError(t("config.grouping.enterDomainFirst"));
      setMessage(null);
      return;
    }
    const rule = `*.*.${domain} => *.${domain}`;
    setDraft((c) => ({
      ...c,
      mode: "balanced",
      aliasRules: appendRuleLine(c.aliasRules, rule),
    }));
    setError(null);
    setMessage(t("config.grouping.addedAliasPreset", { rule }));
  }, [presetDomain, t]);

  const applyBalancedPreset = useCallback(() => {
    setDraft((c) => ({ ...c, mode: "balanced", confidenceThreshold: "80" }));
    setError(null);
    setMessage(t("config.grouping.appliedBalancedGrouping"));
  }, [t]);

  const summaryText = t("config.grouping.summary", {
    mode: t(`config.grouping.modes.${draft.mode}`),
    threshold: normalizeThreshold(draft.confidenceThreshold, "85"),
  });

  return (
    <div className="rounded-xl border border-[var(--panel-border)] bg-[var(--panel-glass)]">
      {/* Header */}
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex items-center justify-between gap-3 w-full px-4 py-3 cursor-pointer"
      >
        <div className="flex items-center gap-2 min-w-0">
          {expanded ? (
            <ChevronDown size={14} className="text-[var(--muted)] shrink-0" />
          ) : (
            <ChevronRight size={14} className="text-[var(--muted)] shrink-0" />
          )}
          <h3 className="text-[13px] font-semibold text-[var(--text)] truncate">
            {t("config.grouping.urlGroupingTitle")}
          </h3>
        </div>
        {!expanded && (
          <span className="text-xs text-[var(--muted)] truncate">
            {summaryText}
          </span>
        )}
      </button>

      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-[var(--line)]">
          {loading && (
            <div className="text-[12px] text-[var(--muted)] pt-3">
              {t("config.grouping.loadingGrouping")}
            </div>
          )}

          {!loading && (
            <>
              {/* Quick presets */}
              <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/60 p-3 mt-3 space-y-2">
                <p className="text-[12px] font-medium text-[var(--text)]">
                  {t("config.grouping.quickPresets")}
                </p>
                <div className="flex flex-col lg:flex-row lg:items-center gap-2">
                  <input
                    type="text"
                    value={presetDomain}
                    onChange={(e) => setPresetDomain(e.target.value)}
                    placeholder={t("config.grouping.domainPlaceholder")}
                    className="h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
                  />
                  <div className="flex items-center gap-2 flex-wrap">
                    <button
                      type="button"
                      onClick={addMiddleLabelPreset}
                      className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
                    >
                      {t("config.grouping.presets.ignoreMiddleLabel")}
                    </button>
                    <button
                      type="button"
                      onClick={applyBalancedPreset}
                      className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
                    >
                      {t("config.grouping.presets.balanced")}
                    </button>
                  </div>
                </div>
                <p className="text-xs text-[var(--muted)]">
                  {t("config.grouping.presetDescription", {
                    ruleFrom: `*.*.${previewDomain}`,
                    ruleTo: `*.${previewDomain}`,
                  })}
                </p>
              </div>

              {/* Mode */}
              <div className="space-y-1.5">
                <label
                  className="text-[12px] font-medium text-[var(--text)]"
                  htmlFor="url-grouping-mode"
                >
                  {t("config.grouping.mode")}
                </label>
                <select
                  id="url-grouping-mode"
                  value={draft.mode}
                  onChange={(e) =>
                    setDraft((c) => ({
                      ...c,
                      mode: normalizeMode(e.target.value),
                    }))
                  }
                  className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] cursor-pointer"
                >
                  {groupingModeValues.map((v) => (
                    <option key={v} value={v}>
                      {t(`config.grouping.modes.${v}`)}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-[var(--muted)]">
                  {t(`config.grouping.groupingModeDesc.${draft.mode}`)}
                </p>
              </div>

              {/* Threshold */}
              <div className="space-y-1.5">
                <label
                  className="text-[12px] font-medium text-[var(--text)]"
                  htmlFor="url-grouping-threshold"
                >
                  {t("config.grouping.confidenceThreshold")} (0-100)
                </label>
                <input
                  id="url-grouping-threshold"
                  type="number"
                  min={0}
                  max={100}
                  value={draft.confidenceThreshold}
                  onChange={(e) =>
                    setDraft((c) => ({
                      ...c,
                      confidenceThreshold: e.target.value,
                    }))
                  }
                  className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
                />
              </div>

              {/* Rules */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <label
                    className="text-[12px] font-medium text-[var(--text)]"
                    htmlFor="url-grouping-alias-rules"
                  >
                    {t("config.grouping.aliasRules")}
                  </label>
                  <textarea
                    id="url-grouping-alias-rules"
                    value={draft.aliasRules}
                    onChange={(e) =>
                      setDraft((c) => ({ ...c, aliasRules: e.target.value }))
                    }
                    rows={7}
                    placeholder={t("config.grouping.aliasRulesPlaceholder")}
                    className="w-full px-3 py-2 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] font-mono focus:outline-none focus:border-[var(--accent)] resize-y"
                  />
                  <p className="text-xs text-[var(--muted)]">
                    {t("config.grouping.aliasRulesDesc")}
                  </p>
                </div>

                <div className="space-y-1.5">
                  <label
                    className="text-[12px] font-medium text-[var(--text)]"
                    htmlFor="url-grouping-never-group"
                  >
                    {t("config.grouping.neverGroupRules")}
                  </label>
                  <textarea
                    id="url-grouping-never-group"
                    value={draft.neverGroupRules}
                    onChange={(e) =>
                      setDraft((c) => ({
                        ...c,
                        neverGroupRules: e.target.value,
                      }))
                    }
                    rows={7}
                    placeholder={t("config.grouping.urlPairPlaceholder")}
                    className="w-full px-3 py-2 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] font-mono focus:outline-none focus:border-[var(--accent)] resize-y"
                  />
                  <p className="text-xs text-[var(--muted)]">
                    {t("config.grouping.neverGroupDesc")}
                  </p>
                </div>
              </div>
            </>
          )}

          {error && <p className="text-[12px] text-[var(--bad)]">{error}</p>}
          {message && (
            <p className="text-[12px] text-[var(--muted)]">{message}</p>
          )}

          {/* Save */}
          {!loading && (
            <div className="flex justify-end pt-1">
              <button
                type="button"
                onClick={() => void save()}
                disabled={saving}
                className="h-7 px-4 rounded bg-[var(--accent)] text-[var(--accent-contrast)] text-xs font-semibold hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50"
              >
                {saving ? t("config.grouping.saving") : t("config.grouping.save")}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// =========================================================================
// Service Merging Card
// =========================================================================

type MergeDraft = {
  mode: GroupingMode;
  confidenceThreshold: string;
  dryRun: boolean;
  aliasRules: string;
  forceMergeRules: string;
  neverMergeRules: string;
};

const defaultMergeDraft: MergeDraft = {
  mode: "conservative",
  confidenceThreshold: "85",
  dryRun: false,
  aliasRules: "",
  forceMergeRules: "",
  neverMergeRules: "",
};

function ServiceMergingCard() {
  const t = useTranslations("services");

  const [expanded, setExpanded] = useState(true);
  const [settings, setSettings] = useState<RuntimeSettingEntry[]>([]);
  const [draft, setDraft] = useState<MergeDraft>(defaultMergeDraft);
  const [presetDomain, setPresetDomain] = useState("simbaslabs.com");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const settingsByKey = useMemo(() => {
    const out = new Map<string, RuntimeSettingEntry>();
    for (const s of settings) out.set(s.key, s);
    return out;
  }, [settings]);

  const applyPayload = useCallback((payload: RuntimeSettingsPayload) => {
    const allSettings = payload.settings ?? [];
    const serviceSettings = allSettings.filter((entry) =>
      (managedMergeKeys as readonly string[]).includes(entry.key)
    );
    setSettings(serviceSettings);

    const byKey = new Map<string, RuntimeSettingEntry>();
    for (const entry of serviceSettings) byKey.set(entry.key, entry);

    setDraft({
      mode: normalizeMode(
        byKey.get(runtimeSettingKeys.servicesMergeMode)?.effective_value ??
          defaultMergeDraft.mode
      ),
      confidenceThreshold: normalizeThreshold(
        byKey.get(runtimeSettingKeys.servicesMergeConfidenceThreshold)
          ?.effective_value ?? defaultMergeDraft.confidenceThreshold,
        defaultMergeDraft.confidenceThreshold
      ),
      dryRun: parseBool(
        byKey.get(runtimeSettingKeys.servicesMergeDryRun)?.effective_value ??
          String(defaultMergeDraft.dryRun),
        defaultMergeDraft.dryRun
      ),
      aliasRules:
        byKey.get(runtimeSettingKeys.servicesMergeAliasRules)?.effective_value ??
        defaultMergeDraft.aliasRules,
      forceMergeRules:
        byKey.get(runtimeSettingKeys.servicesForceMergeRules)?.effective_value ??
        defaultMergeDraft.forceMergeRules,
      neverMergeRules:
        byKey.get(runtimeSettingKeys.servicesNeverMergeRules)?.effective_value ??
        defaultMergeDraft.neverMergeRules,
    });
  }, []);

  const loadSettings = useCallback(async () => {
    setLoading(true);
    setError(null);
    setMessage(null);
    try {
      const response = await fetch("/api/settings/runtime", {
        cache: "no-store",
      });
      const payload = (await response.json()) as RuntimeSettingsPayload;
      if (!response.ok) {
        throw new Error(
          payload.error || `Failed to load settings (HTTP ${response.status})`
        );
      }
      applyPayload(payload);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to load merge settings"
      );
    } finally {
      setLoading(false);
    }
  }, [applyPayload]);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  const save = useCallback(async () => {
    setSaving(true);
    setError(null);
    setMessage(null);
    const values: Record<string, string> = {
      [runtimeSettingKeys.servicesMergeMode]: draft.mode,
      [runtimeSettingKeys.servicesMergeConfidenceThreshold]: normalizeThreshold(
        draft.confidenceThreshold,
        defaultMergeDraft.confidenceThreshold
      ),
      [runtimeSettingKeys.servicesMergeDryRun]: String(draft.dryRun),
      [runtimeSettingKeys.servicesMergeAliasRules]: draft.aliasRules.trim(),
      [runtimeSettingKeys.servicesForceMergeRules]:
        draft.forceMergeRules.trim(),
      [runtimeSettingKeys.servicesNeverMergeRules]:
        draft.neverMergeRules.trim(),
    };
    try {
      const response = await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ values }),
      });
      const payload = (await response.json()) as RuntimeSettingsPayload;
      if (!response.ok) {
        throw new Error(
          payload.error || `Failed to save settings (HTTP ${response.status})`
        );
      }
      applyPayload(payload);
      setMessage(t("config.grouping.saved"));
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to save merge settings"
      );
    } finally {
      setSaving(false);
    }
  }, [applyPayload, draft, t]);

  const previewDomain = normalizeBaseDomain(presetDomain) || "simbaslabs.com";

  const addMiddleLabelPreset = useCallback(() => {
    const domain = normalizeBaseDomain(presetDomain);
    if (!domain) {
      setError(t("config.grouping.enterDomainFirst"));
      setMessage(null);
      return;
    }
    const rule = `*.*.${domain} => *.${domain}`;
    setDraft((c) => ({
      ...c,
      mode: "balanced",
      dryRun: false,
      aliasRules: appendRuleLine(c.aliasRules, rule),
    }));
    setError(null);
    setMessage(t("config.grouping.addedAliasPreset", { rule }));
  }, [presetDomain, t]);

  const applySafePreset = useCallback(() => {
    setDraft((c) => ({
      ...c,
      mode: "conservative",
      confidenceThreshold: "90",
      dryRun: true,
    }));
    setError(null);
    setMessage(t("config.grouping.appliedSafeDryRun"));
  }, [t]);

  const applyLivePreset = useCallback(() => {
    setDraft((c) => ({
      ...c,
      mode: "balanced",
      confidenceThreshold: "80",
      dryRun: false,
    }));
    setError(null);
    setMessage(t("config.grouping.appliedBalancedLiveMerge"));
  }, [t]);

  function SourceBadge({ settingKey }: { settingKey: string }) {
    const entry = settingsByKey.get(settingKey);
    return (
      <span
        className={`text-[10px] px-1.5 py-0.5 rounded ${sourceClassName(entry?.source)}`}
      >
        {sourceLabel(entry?.source)}
      </span>
    );
  }

  const summaryText = t("config.grouping.summary", {
    mode: t(`config.grouping.modes.${draft.mode}`),
    threshold: normalizeThreshold(
      draft.confidenceThreshold,
      defaultMergeDraft.confidenceThreshold
    ),
  });

  return (
    <div className="rounded-xl border border-[var(--panel-border)] bg-[var(--panel-glass)]">
      {/* Header */}
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex items-center justify-between gap-3 w-full px-4 py-3 cursor-pointer"
      >
        <div className="flex items-center gap-2 min-w-0">
          {expanded ? (
            <ChevronDown size={14} className="text-[var(--muted)] shrink-0" />
          ) : (
            <ChevronRight size={14} className="text-[var(--muted)] shrink-0" />
          )}
          <h3 className="text-[13px] font-semibold text-[var(--text)] truncate">
            {t("config.grouping.serviceMergingTitle")}
          </h3>
        </div>
        {!expanded && (
          <span className="text-xs text-[var(--muted)] truncate">
            {summaryText}
          </span>
        )}
      </button>

      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-[var(--line)]">
          {loading && (
            <div className="text-[12px] text-[var(--muted)] pt-3">
              {t("config.grouping.loadingMerge")}
            </div>
          )}

          {!loading && (
            <>
              {/* Quick presets */}
              <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/60 p-3 mt-3 space-y-2">
                <p className="text-[12px] font-medium text-[var(--text)]">
                  {t("config.grouping.quickPresets")}
                </p>
                <div className="flex flex-col lg:flex-row lg:items-center gap-2">
                  <input
                    type="text"
                    value={presetDomain}
                    onChange={(e) => setPresetDomain(e.target.value)}
                    placeholder={t("config.grouping.domainPlaceholder")}
                    className="h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
                  />
                  <div className="flex items-center gap-2 flex-wrap">
                    <button
                      type="button"
                      onClick={addMiddleLabelPreset}
                      className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
                    >
                      {t("config.grouping.presets.ignoreMiddleLabel")}
                    </button>
                    <button
                      type="button"
                      onClick={applySafePreset}
                      className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
                    >
                      {t("config.grouping.presets.safe")}
                    </button>
                    <button
                      type="button"
                      onClick={applyLivePreset}
                      className="h-7 px-2.5 rounded border border-[var(--line)] text-xs font-medium text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer"
                    >
                      {t("config.grouping.presets.live")}
                    </button>
                  </div>
                </div>
                <p className="text-xs text-[var(--muted)]">
                  {t("config.grouping.presetDescription", {
                    ruleFrom: `*.*.${previewDomain}`,
                    ruleTo: `*.${previewDomain}`,
                  })}
                </p>
              </div>

              {/* Mode */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between gap-2">
                  <label
                    className="text-[12px] font-medium text-[var(--text)]"
                    htmlFor="merge-mode"
                  >
                    {t("config.grouping.mode")}
                  </label>
                  <SourceBadge
                    settingKey={runtimeSettingKeys.servicesMergeMode}
                  />
                </div>
                <select
                  id="merge-mode"
                  value={draft.mode}
                  onChange={(e) =>
                    setDraft((c) => ({
                      ...c,
                      mode: normalizeMode(e.target.value),
                    }))
                  }
                  className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)] cursor-pointer"
                >
                  {mergeModeValues.map((v) => (
                    <option key={v} value={v}>
                      {t(`config.grouping.modes.${v}`)}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-[var(--muted)]">
                  {t(`config.grouping.mergeModeDesc.${draft.mode}`)}
                </p>
              </div>

              {/* Threshold */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between gap-2">
                  <label
                    className="text-[12px] font-medium text-[var(--text)]"
                    htmlFor="merge-threshold"
                  >
                    {t("config.grouping.confidenceThreshold")} (0-100)
                  </label>
                  <SourceBadge
                    settingKey={
                      runtimeSettingKeys.servicesMergeConfidenceThreshold
                    }
                  />
                </div>
                <input
                  id="merge-threshold"
                  type="number"
                  min={0}
                  max={100}
                  value={draft.confidenceThreshold}
                  onChange={(e) =>
                    setDraft((c) => ({
                      ...c,
                      confidenceThreshold: e.target.value,
                    }))
                  }
                  className="w-full h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
                />
              </div>

              {/* Dry run */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between gap-2">
                  <label
                    className="text-[12px] font-medium text-[var(--text)]"
                    htmlFor="merge-dry-run"
                  >
                    {t("config.grouping.dryRun")}
                  </label>
                  <SourceBadge
                    settingKey={runtimeSettingKeys.servicesMergeDryRun}
                  />
                </div>
                <label
                  htmlFor="merge-dry-run"
                  className="h-8 px-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] inline-flex items-center gap-2 cursor-pointer select-none"
                >
                  <input
                    id="merge-dry-run"
                    type="checkbox"
                    checked={draft.dryRun}
                    onChange={(e) =>
                      setDraft((c) => ({ ...c, dryRun: e.target.checked }))
                    }
                    className="accent-[var(--accent)]"
                  />
                  {t("config.grouping.dryRunLabel")}
                </label>
              </div>

              {/* Rules grid */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {/* Alias rules */}
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <label
                      className="text-[12px] font-medium text-[var(--text)]"
                      htmlFor="merge-alias-rules"
                    >
                      {t("config.grouping.aliasRules")}
                    </label>
                    <SourceBadge
                      settingKey={runtimeSettingKeys.servicesMergeAliasRules}
                    />
                  </div>
                  <textarea
                    id="merge-alias-rules"
                    value={draft.aliasRules}
                    onChange={(e) =>
                      setDraft((c) => ({ ...c, aliasRules: e.target.value }))
                    }
                    rows={7}
                    placeholder={t("config.grouping.aliasRulesPlaceholder")}
                    className="w-full px-3 py-2 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] font-mono focus:outline-none focus:border-[var(--accent)] resize-y"
                  />
                  <p className="text-xs text-[var(--muted)]">
                    {t("config.grouping.aliasRulesDesc")}
                  </p>
                </div>

                {/* Force-merge + Never-merge stacked */}
                <div className="space-y-3">
                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between gap-2">
                      <label
                        className="text-[12px] font-medium text-[var(--text)]"
                        htmlFor="merge-force-rules"
                      >
                        {t("config.grouping.forceMergeRules")}
                      </label>
                      <SourceBadge
                        settingKey={runtimeSettingKeys.servicesForceMergeRules}
                      />
                    </div>
                    <textarea
                      id="merge-force-rules"
                      value={draft.forceMergeRules}
                      onChange={(e) =>
                        setDraft((c) => ({
                          ...c,
                          forceMergeRules: e.target.value,
                        }))
                      }
                      rows={3}
                      placeholder={t("config.grouping.urlPairPlaceholder")}
                      className="w-full px-3 py-2 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] font-mono focus:outline-none focus:border-[var(--accent)] resize-y"
                    />
                    <p className="text-xs text-[var(--muted)]">
                      {t("config.grouping.forceMergeDesc")}
                    </p>
                  </div>

                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between gap-2">
                      <label
                        className="text-[12px] font-medium text-[var(--text)]"
                        htmlFor="merge-never-rules"
                      >
                        {t("config.grouping.neverMergeRules")}
                      </label>
                      <SourceBadge
                        settingKey={runtimeSettingKeys.servicesNeverMergeRules}
                      />
                    </div>
                    <textarea
                      id="merge-never-rules"
                      value={draft.neverMergeRules}
                      onChange={(e) =>
                        setDraft((c) => ({
                          ...c,
                          neverMergeRules: e.target.value,
                        }))
                      }
                      rows={3}
                      placeholder={t("config.grouping.urlPairPlaceholder")}
                      className="w-full px-3 py-2 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] font-mono focus:outline-none focus:border-[var(--accent)] resize-y"
                    />
                    <p className="text-xs text-[var(--muted)]">
                      {t("config.grouping.neverMergeDesc")}
                    </p>
                  </div>
                </div>
              </div>
            </>
          )}

          {error && <p className="text-[12px] text-[var(--bad)]">{error}</p>}
          {message && (
            <p className="text-[12px] text-[var(--muted)]">{message}</p>
          )}

          {/* Save */}
          {!loading && (
            <div className="flex justify-end pt-1">
              <button
                type="button"
                onClick={() => void save()}
                disabled={saving}
                className="h-7 px-4 rounded bg-[var(--accent)] text-[var(--accent-contrast)] text-xs font-semibold hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50"
              >
                {saving ? t("config.grouping.saving") : t("config.grouping.save")}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// =========================================================================
// Exported Tab Component
// =========================================================================

export default function GroupingMergeTab() {
  return (
    <div className="space-y-4">
      <URLGroupingCard />
      <ServiceMergingCard />
    </div>
  );
}
