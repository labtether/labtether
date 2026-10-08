"use client";

import { useTranslations } from "next-intl";

/* ── scope categories (mirrors internal/apikeys/scope.go knownScopeCategories) ── */

export const SCOPE_GROUPS: { label: string; scopes: string[] }[] = [
  { label: "Assets & Inventory", scopes: ["assets", "groups", "topology", "discovery"] },
  { label: "Operations", scopes: ["shell", "files", "services", "processes", "cron"] },
  { label: "Monitoring", scopes: ["alerts", "metrics", "logs", "incidents", "notifications"] },
  { label: "System", scopes: ["network", "disks", "packages", "users", "settings", "updates"] },
  { label: "Integrations", scopes: ["docker", "connectors", "homeassistant", "agents", "collectors", "web-services"] },
  { label: "Automation", scopes: ["webhooks", "schedules", "actions", "events", "bulk"] },
  { label: "Platform", scopes: ["hub", "failover", "terminal", "search", "dead-letters", "credentials", "audit"] },
];

export const ALL_SCOPES = SCOPE_GROUPS.flatMap((g) => g.scopes);

/* ── scope selector ── */

export type ScopeSelectorProps = {
  fullAccess: boolean;
  onFullAccessChange: (v: boolean) => void;
  selectedScopes: Set<string>;
  onToggleScope: (scope: string) => void;
  onToggleGroup: (scopes: string[], allSelected: boolean) => void;
};

export function ScopeSelector({ fullAccess, onFullAccessChange, selectedScopes, onToggleScope, onToggleGroup }: ScopeSelectorProps) {
  const t = useTranslations("settings");

  return (
    <div className="space-y-2">
      <label className="flex items-center gap-2 text-xs text-[var(--text)] cursor-pointer select-none">
        <input
          type="checkbox"
          checked={fullAccess}
          onChange={(e) => onFullAccessChange(e.target.checked)}
          className="accent-[var(--accent)]"
        />
        {t("apiKeys.scopesFullAccess")}
      </label>
      {!fullAccess && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 pl-1">
          {SCOPE_GROUPS.map((group) => {
            const allSelected = group.scopes.every((s) => selectedScopes.has(s));
            const someSelected = group.scopes.some((s) => selectedScopes.has(s));
            return (
              <div key={group.label} className="space-y-1">
                <label className="flex items-center gap-2 text-xs font-medium text-[var(--text)] cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    ref={(el) => {
                      if (el) el.indeterminate = someSelected && !allSelected;
                    }}
                    onChange={() => onToggleGroup(group.scopes, allSelected)}
                    className="accent-[var(--accent)]"
                  />
                  {group.label}
                </label>
                <div className="flex flex-wrap gap-1 pl-5">
                  {group.scopes.map((scope) => (
                    <button
                      key={scope}
                      type="button"
                      onClick={() => onToggleScope(scope)}
                      className={`px-1.5 py-0.5 rounded text-[10px] font-mono border cursor-pointer transition-colors ${
                        selectedScopes.has(scope)
                          ? "bg-[var(--accent)]/15 border-[var(--accent)]/40 text-[var(--accent)]"
                          : "bg-transparent border-[var(--line)] text-[var(--muted)] hover:border-[var(--text)]"
                      }`}
                    >
                      {scope}
                    </button>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export type AssetOption = {
  id: string;
  name: string;
};

export type AssetSelectorProps = {
  assets: AssetOption[];
  inputName: string;
  restricted: boolean;
  onRestrictedChange: (value: boolean) => void;
  selectedAssetIDs: Set<string>;
  onToggleAsset: (assetID: string) => void;
};

export function AssetSelector({
  assets,
  inputName,
  restricted,
  onRestrictedChange,
  selectedAssetIDs,
  onToggleAsset,
}: AssetSelectorProps) {
  const t = useTranslations("settings");

  return (
    <fieldset className="space-y-2">
      <legend className="block text-[10px] uppercase tracking-wider text-[var(--muted)] mb-1.5">
        {t("apiKeys.allowedAssets")}
      </legend>
      <label className="flex items-center gap-2 text-xs text-[var(--text)] cursor-pointer select-none">
        <input
          type="radio"
          name={inputName}
          checked={!restricted}
          onChange={() => onRestrictedChange(false)}
          className="accent-[var(--accent)]"
        />
        {t("apiKeys.allowedAssetsAll")}
      </label>
      <label className="flex items-center gap-2 text-xs text-[var(--text)] cursor-pointer select-none">
        <input
          type="radio"
          name={inputName}
          checked={restricted}
          onChange={() => onRestrictedChange(true)}
          className="accent-[var(--accent)]"
        />
        {t("apiKeys.allowedAssetsSelect")}
      </label>
      {restricted && (
        assets.length > 0 ? (
          <div className="grid max-h-48 grid-cols-1 gap-1 overflow-y-auto rounded-lg border border-[var(--line)] bg-[var(--surface)] p-2 sm:grid-cols-2">
            {assets.map((asset) => (
              <label
                key={asset.id}
                className="flex min-w-0 cursor-pointer items-center gap-2 rounded px-1.5 py-1 text-xs text-[var(--text)] hover:bg-[var(--surface-hover)]"
              >
                <input
                  type="checkbox"
                  checked={selectedAssetIDs.has(asset.id)}
                  onChange={() => onToggleAsset(asset.id)}
                  className="shrink-0 accent-[var(--accent)]"
                />
                <span className="min-w-0 truncate" title={`${asset.name} (${asset.id})`}>
                  {asset.name}
                  <span className="ml-1 font-mono text-[10px] text-[var(--muted)]">{asset.id}</span>
                </span>
              </label>
            ))}
          </div>
        ) : (
          <p className="pl-5 text-xs text-[var(--muted)]">{t("apiKeys.allowedAssetsEmpty")}</p>
        )
      )}
      {restricted && selectedAssetIDs.size === 0 && (
        <p className="pl-5 text-xs text-[var(--bad)]">{t("apiKeys.allowedAssetsRequired")}</p>
      )}
    </fieldset>
  );
}
