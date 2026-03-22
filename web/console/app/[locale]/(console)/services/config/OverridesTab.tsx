"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { RotateCcw, SlidersHorizontal } from "lucide-react";
import { Button } from "../../../../components/ui/Button";
import { EmptyState } from "../../../../components/ui/EmptyState";
import { useWebServices, type WebServiceOverride } from "../../../../hooks/useWebServices";
import { useStatus } from "../../../../contexts/StatusContext";

type OverrideFieldKey = "name" | "url" | "category" | "icon" | "tags" | "hidden";

function getOverriddenFields(override: WebServiceOverride): OverrideFieldKey[] {
  const fields: OverrideFieldKey[] = [];
  if (override.name_override) fields.push("name");
  if (override.url_override) fields.push("url");
  if (override.category_override) fields.push("category");
  if (override.icon_key_override) fields.push("icon");
  if (override.tags_override) fields.push("tags");
  if (override.hidden) fields.push("hidden");
  return fields;
}

function overrideKey(override: WebServiceOverride): string {
  return `${override.host_asset_id}::${override.service_id}`;
}

function formatTimestamp(raw?: string): string {
  if (!raw) return "--";
  try {
    const date = new Date(raw);
    if (Number.isNaN(date.getTime())) return "--";
    return date.toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return "--";
  }
}

export default function OverridesTab() {
  const t = useTranslations("services");
  const { status } = useStatus();
  const {
    services,
    listServiceOverrides,
    deleteServiceOverride,
    refresh,
  } = useWebServices({ includeHidden: true });

  const [overrides, setOverrides] = useState<WebServiceOverride[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [resetting, setResetting] = useState(false);

  // Filter state
  const [hostFilter, setHostFilter] = useState("");
  const [fieldFilter, setFieldFilter] = useState<OverrideFieldKey | "">("");

  const assets = useMemo(() => status?.assets ?? [], [status?.assets]);

  const assetNameMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const asset of assets) {
      map.set(asset.id, asset.name);
    }
    return map;
  }, [assets]);

  const fetchOverrides = useCallback(async () => {
    try {
      const result = await listServiceOverrides();
      setOverrides(result);
    } catch {
      // Silently handle -- the hook reports errors
    } finally {
      setLoading(false);
    }
  }, [listServiceOverrides]);

  useEffect(() => {
    void fetchOverrides();
  }, [fetchOverrides]);

  // Unique hosts present in overrides for the filter dropdown
  const hostOptions = useMemo(() => {
    const ids = new Set<string>();
    for (const o of overrides) {
      if (o.host_asset_id) ids.add(o.host_asset_id);
    }
    return Array.from(ids).sort((a, b) => {
      const nameA = assetNameMap.get(a) ?? a;
      const nameB = assetNameMap.get(b) ?? b;
      return nameA.localeCompare(nameB);
    });
  }, [overrides, assetNameMap]);

  // Filtered overrides
  const filteredOverrides = useMemo(() => {
    let list = overrides;
    if (hostFilter) {
      list = list.filter((o) => o.host_asset_id === hostFilter);
    }
    if (fieldFilter) {
      list = list.filter((o) => getOverriddenFields(o).includes(fieldFilter));
    }
    return list;
  }, [overrides, hostFilter, fieldFilter]);

  const allSelected =
    filteredOverrides.length > 0 &&
    filteredOverrides.every((o) => selected.has(overrideKey(o)));

  const toggleAll = useCallback(() => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      const next = new Set<string>();
      for (const o of filteredOverrides) {
        next.add(overrideKey(o));
      }
      setSelected(next);
    }
  }, [allSelected, filteredOverrides]);

  const toggleOne = useCallback((override: WebServiceOverride) => {
    setSelected((prev) => {
      const next = new Set(prev);
      const key = overrideKey(override);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const handleResetOne = useCallback(
    async (override: WebServiceOverride) => {
      if (!window.confirm(t("config.overrides.resetConfirm"))) return;
      setResetting(true);
      try {
        await deleteServiceOverride(override.host_asset_id, override.service_id);
        await fetchOverrides();
        await refresh();
        setSelected((prev) => {
          const next = new Set(prev);
          next.delete(overrideKey(override));
          return next;
        });
      } catch {
        // Error handled by hook
      } finally {
        setResetting(false);
      }
    },
    [deleteServiceOverride, fetchOverrides, refresh, t]
  );

  const handleBulkReset = useCallback(async () => {
    const targets = filteredOverrides.filter((o) => selected.has(overrideKey(o)));
    if (targets.length === 0) return;
    if (
      !window.confirm(
        t("config.overrides.bulkResetConfirm", { count: targets.length })
      )
    )
      return;
    setResetting(true);
    let allSucceeded = true;
    try {
      for (const override of targets) {
        await deleteServiceOverride(override.host_asset_id, override.service_id);
      }
    } catch {
      allSucceeded = false;
    } finally {
      await fetchOverrides();
      await refresh();
      if (allSucceeded) {
        setSelected(new Set());
      }
      setResetting(false);
    }
  }, [filteredOverrides, selected, deleteServiceOverride, fetchOverrides, refresh, t]);

  const selectedCount = filteredOverrides.filter((o) =>
    selected.has(overrideKey(o))
  ).length;

  function originalServiceName(override: WebServiceOverride): string {
    return (
      services.find((s) => s.id === override.service_id)?.name ??
      override.service_id
    );
  }

  function hostLabel(override: WebServiceOverride): string {
    if (!override.host_asset_id) return "--";
    return assetNameMap.get(override.host_asset_id) ?? override.host_asset_id;
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-sm text-[var(--muted)]">{t("config.overrides.loading")}</div>
      </div>
    );
  }

  if (overrides.length === 0) {
    return (
      <EmptyState
        icon={SlidersHorizontal}
        title={t("config.tabs.overrides")}
        description={t("config.overrides.emptyState")}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Toolbar: filters + bulk action */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Host filter */}
        <select
          value={hostFilter}
          onChange={(e) => setHostFilter(e.target.value)}
          className="rounded-lg border border-[var(--control-border)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--control-focus-ring)]"
        >
          <option value="">{t("config.overrides.host")}: --</option>
          {hostOptions.map((id) => (
            <option key={id} value={id}>
              {assetNameMap.get(id) ?? id}
            </option>
          ))}
        </select>

        {/* Field filter */}
        <select
          value={fieldFilter}
          onChange={(e) =>
            setFieldFilter(e.target.value as OverrideFieldKey | "")
          }
          className="rounded-lg border border-[var(--control-border)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--control-focus-ring)]"
        >
          <option value="">
            {t("config.overrides.overriddenFields")}: --
          </option>
          {(
            ["name", "url", "category", "icon", "tags", "hidden"] as const
          ).map((field) => (
            <option key={field} value={field}>
              {t(`config.overrides.fields.${field}`)}
            </option>
          ))}
        </select>

        {/* Spacer */}
        <div className="flex-1" />

        {/* Bulk reset */}
        {selectedCount > 0 && (
          <Button
            variant="danger"
            size="sm"
            loading={resetting}
            onClick={handleBulkReset}
          >
            <RotateCcw className="h-3.5 w-3.5" />
            {t("config.overrides.resetSelected")} ({selectedCount})
          </Button>
        )}
      </div>

      {/* Table */}
      {filteredOverrides.length === 0 ? (
        <EmptyState
          icon={SlidersHorizontal}
          title={t("config.tabs.overrides")}
          description={t("config.overrides.emptyState")}
        />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-[var(--line)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--line)] bg-[var(--surface)]">
                <th className="w-8 px-3 py-2">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={toggleAll}
                    className="accent-[var(--accent)] cursor-pointer"
                  />
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.overrides.serviceName")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.overrides.host")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.overrides.overriddenFields")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.overrides.lastUpdated")}
                </th>
                <th className="text-right px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.overrides.actions")}
                </th>
              </tr>
            </thead>
            <tbody>
              {filteredOverrides.map((override) => {
                const key = overrideKey(override);
                const fields = getOverriddenFields(override);
                return (
                  <tr
                    key={key}
                    className="border-b border-[var(--line)] last:border-b-0 hover:bg-[var(--hover)] transition-colors"
                  >
                    <td className="px-3 py-2">
                      <input
                        type="checkbox"
                        checked={selected.has(key)}
                        onChange={() => toggleOne(override)}
                        className="accent-[var(--accent)] cursor-pointer"
                      />
                    </td>
                    <td className="px-3 py-2 text-[var(--text)] font-medium">
                      {originalServiceName(override)}
                    </td>
                    <td className="px-3 py-2 text-[var(--muted)]">
                      {hostLabel(override)}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex flex-wrap gap-1">
                        {fields.map((field) => (
                          <span
                            key={field}
                            className="inline-flex items-center rounded-full bg-[var(--accent)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--accent)]"
                          >
                            {t(`config.overrides.fields.${field}`)}
                          </span>
                        ))}
                        {fields.length === 0 && (
                          <span className="text-xs text-[var(--muted)]">
                            --
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-3 py-2 text-[var(--muted)] text-xs">
                      {formatTimestamp(override.updated_at)}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <button
                        type="button"
                        className="inline-flex items-center gap-1 px-2 py-1 rounded text-xs text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-40 disabled:pointer-events-none"
                        onClick={() => handleResetOne(override)}
                        disabled={resetting}
                        title={t("config.overrides.resetToAuto")}
                      >
                        <RotateCcw className="h-3 w-3" />
                        {t("config.overrides.resetToAuto")}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
