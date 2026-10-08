"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import type { ApiKeyInfo, UpdateKeyRequest } from "../../../../hooks/useApiKeys";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";
import { type AssetOption, ALL_SCOPES, ScopeSelector, AssetSelector } from "./ApiKeyAccessSelectors";
import { expiryToIso } from "./apiKeyPresentation";

export type EditKeyModalProps = {
  keyInfo: ApiKeyInfo;
  assets: AssetOption[];
  onClose: () => void;
  onConfirm: (request: UpdateKeyRequest) => Promise<void>;
};

export function EditKeyModal({ keyInfo, assets, onClose, onConfirm }: EditKeyModalProps) {
  const t = useTranslations("settings");
  const initialFullAccess = keyInfo.scopes.includes("*");
  const [name, setName] = useState(keyInfo.name);
  const [fullAccess, setFullAccess] = useState(initialFullAccess);
  const [selectedScopes, setSelectedScopes] = useState<Set<string>>(
    new Set(initialFullAccess ? ALL_SCOPES : keyInfo.scopes),
  );
  const [restrictAssets, setRestrictAssets] = useState((keyInfo.allowed_assets?.length ?? 0) > 0);
  const [selectedAssetIDs, setSelectedAssetIDs] = useState<Set<string>>(new Set(keyInfo.allowed_assets ?? []));
  const [expiry, setExpiry] = useState("unchanged");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const toggleScope = (scope: string) => {
    setSelectedScopes((previous) => {
      const next = new Set(previous);
      if (next.has(scope)) next.delete(scope);
      else next.add(scope);
      return next;
    });
  };
  const toggleGroup = (scopes: string[], allSelected: boolean) => {
    setSelectedScopes((previous) => {
      const next = new Set(previous);
      for (const scope of scopes) {
        if (allSelected) next.delete(scope);
        else next.add(scope);
      }
      return next;
    });
  };
  const toggleAsset = (assetID: string) => {
    setSelectedAssetIDs((previous) => {
      const next = new Set(previous);
      if (next.has(assetID)) next.delete(assetID);
      else next.add(assetID);
      return next;
    });
  };
  const canSave = name.trim().length > 0
    && (fullAccess || selectedScopes.size > 0)
    && (!restrictAssets || selectedAssetIDs.size > 0);

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    setError("");
    const request: UpdateKeyRequest = {
      name: name.trim(),
      scopes: fullAccess ? ["*"] : Array.from(selectedScopes).sort(),
      allowed_assets: restrictAssets ? Array.from(selectedAssetIDs).sort() : [],
    };
    if (expiry !== "unchanged") request.expires_at = expiryToIso(expiry);
    try {
      await onConfirm(request);
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "Failed to update key."));
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="api-key-edit-title"
        className="max-h-[90vh] w-[42rem] max-w-[96vw] overflow-y-auto"
      >
        <Card className="space-y-4">
          <h3 id="api-key-edit-title" className="text-sm font-medium text-[var(--text)]">{t("apiKeys.editTitle")}</h3>
          <div>
            <label htmlFor="api-key-edit-name" className="mb-1 block text-[10px] uppercase tracking-wider text-[var(--muted)]">
              {t("apiKeys.name")}
            </label>
            <input
              id="api-key-edit-name"
              type="text"
              value={name}
              maxLength={120}
              onChange={(event) => setName(event.target.value)}
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
            />
          </div>
          <div>
            <p className="mb-1.5 text-[10px] uppercase tracking-wider text-[var(--muted)]">{t("apiKeys.scopes")}</p>
            <ScopeSelector
              fullAccess={fullAccess}
              onFullAccessChange={setFullAccess}
              selectedScopes={selectedScopes}
              onToggleScope={toggleScope}
              onToggleGroup={toggleGroup}
            />
          </div>
          <AssetSelector
            assets={assets}
            inputName={`api-key-edit-assets-mode-${keyInfo.id}`}
            restricted={restrictAssets}
            onRestrictedChange={setRestrictAssets}
            selectedAssetIDs={selectedAssetIDs}
            onToggleAsset={toggleAsset}
          />
          <div>
            <label htmlFor="api-key-edit-expiry" className="mb-1 block text-[10px] uppercase tracking-wider text-[var(--muted)]">
              {t("apiKeys.expiresAt")}
            </label>
            <select
              id="api-key-edit-expiry"
              value={expiry}
              onChange={(event) => setExpiry(event.target.value)}
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)] sm:w-48"
            >
              <option value="unchanged">{t("apiKeys.expiresUnchanged")}</option>
              <option value="30d">{t("apiKeys.expires30d")}</option>
              <option value="90d">{t("apiKeys.expires90d")}</option>
              <option value="1y">{t("apiKeys.expires1y")}</option>
              <option value="never">{t("apiKeys.expiresNever")}</option>
            </select>
          </div>
          {error ? <p className="text-xs text-[var(--bad)]" role="alert">{error}</p> : null}
          <div className="flex justify-end gap-2">
            <Button variant="secondary" disabled={saving} onClick={onClose}>{t("apiKeys.cancel")}</Button>
            <Button variant="primary" loading={saving} disabled={!canSave} onClick={() => { void handleSave(); }}>
              {t("apiKeys.saveChanges")}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
