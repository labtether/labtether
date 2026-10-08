"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { ChevronDown, ChevronRight } from "lucide-react";
import { type GroupingMode, normalizeMode, normalizeThreshold, normalizeBaseDomain, appendRuleLine } from "./groupingConfigHelpers";

// ---------------------------------------------------------------------------
// Mode option descriptors
// ---------------------------------------------------------------------------

export const groupingModeValues: GroupingMode[] = ["off", "conservative", "balanced", "aggressive"];

// =========================================================================
// URL Grouping Card
// =========================================================================

export type GroupingDraft = {
  mode: GroupingMode;
  confidenceThreshold: string;
  aliasRules: string;
  neverGroupRules: string;
};

export const defaultGroupingDraft: GroupingDraft = {
  mode: "balanced",
  confidenceThreshold: "85",
  aliasRules: "",
  neverGroupRules: "",
};

export function URLGroupingCard() {
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
