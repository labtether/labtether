"use client";

import { useCallback, useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { useAuth } from "../../../../contexts/AuthContext";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useApiKeys } from "../../../../hooks/useApiKeys";
import type { ApiKeyInfo, CreateKeyRequest, CreatedKeyResponse, UpdateKeyRequest } from "../../../../hooks/useApiKeys";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";
import { type AssetOption, ALL_SCOPES, ScopeSelector, AssetSelector } from "./ApiKeyAccessSelectors";
import { expiryToIso } from "./apiKeyPresentation";
import { KeyRow } from "./ApiKeyRow";
import { KeyRevealModal, RevokeConfirmModal } from "./ApiKeySecurityDialogs";
import { EditKeyModal } from "./EditKeyModal";

/* ── main card ── */

type Dialog =
  | { type: "reveal"; created: CreatedKeyResponse }
  | { type: "edit"; keyInfo: ApiKeyInfo }
  | { type: "revoke"; keyInfo: ApiKeyInfo }
  | null;

export function ApiKeysCard() {
  const t = useTranslations("settings");
  const { user } = useAuth();
  const status = useFastStatus();
  const { keys, loading, error, createKey, updateKey, revokeKey } = useApiKeys();

  const assets = useMemo<AssetOption[]>(() => (
    (status?.assets ?? [])
      .map((asset) => ({ id: asset.id, name: asset.name || asset.id }))
      .sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id))
  ), [status?.assets]);
  const assetNameByID = useMemo(
    () => new Map(assets.map((asset) => [asset.id, asset.name])),
    [assets],
  );

  /* create form state */
  const [name, setName] = useState("");
  const [role, setRole] = useState("operator");
  const [expiry, setExpiry] = useState("90d");
  const [fullAccess, setFullAccess] = useState(true);
  const [selectedScopes, setSelectedScopes] = useState<Set<string>>(new Set(ALL_SCOPES));
  const [restrictAssets, setRestrictAssets] = useState(false);
  const [selectedAssetIDs, setSelectedAssetIDs] = useState<Set<string>>(new Set());
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  const [dialog, setDialog] = useState<Dialog>(null);

  /* admin gate */
  const isAdmin = user?.role === "owner" || user?.role === "admin";

  const handleToggleScope = useCallback((scope: string) => {
    setSelectedScopes((prev) => {
      const next = new Set(prev);
      if (next.has(scope)) next.delete(scope);
      else next.add(scope);
      return next;
    });
  }, []);

  const handleToggleGroup = useCallback((scopes: string[], allSelected: boolean) => {
    setSelectedScopes((prev) => {
      const next = new Set(prev);
      for (const s of scopes) {
        if (allSelected) next.delete(s);
        else next.add(s);
      }
      return next;
    });
  }, []);

  const handleToggleAsset = useCallback((assetID: string) => {
    setSelectedAssetIDs((previous) => {
      const next = new Set(previous);
      if (next.has(assetID)) next.delete(assetID);
      else next.add(assetID);
      return next;
    });
  }, []);

  const canCreate = name.trim().length > 0
    && (fullAccess || selectedScopes.size > 0)
    && (!restrictAssets || selectedAssetIDs.size > 0);

  const handleCreate = async () => {
    if (!canCreate) return;
    setCreating(true);
    setCreateError("");
    try {
      const req: CreateKeyRequest = {
        name: name.trim(),
        role,
        scopes: fullAccess ? ["*"] : Array.from(selectedScopes),
        allowed_assets: restrictAssets ? Array.from(selectedAssetIDs).sort() : [],
        expires_at: expiryToIso(expiry),
      };
      const created = await createKey(req);
      setDialog({ type: "reveal", created });
      /* reset form */
      setName("");
      setRole("operator");
      setExpiry("90d");
      setFullAccess(true);
      setSelectedScopes(new Set(ALL_SCOPES));
      setRestrictAssets(false);
      setSelectedAssetIDs(new Set());
    } catch (err) {
      setCreateError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "Failed to create key."));
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (keyInfo: ApiKeyInfo) => {
    await revokeKey(keyInfo.id);
    setDialog(null);
  };

  const handleUpdate = async (keyInfo: ApiKeyInfo, request: UpdateKeyRequest) => {
    await updateKey(keyInfo.id, request);
    setDialog(null);
  };

  /* responsive header columns for key table */
  const tableHeader = useMemo(
    () => (
      <div className="grid min-w-[58rem] grid-cols-[5rem_1fr_4rem_1fr_8rem_5.5rem_5.5rem_4rem] items-center gap-2 bg-[var(--surface)] px-3 py-2 text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">
        <span>{t("apiKeys.colPrefix")}</span>
        <span>{t("apiKeys.colName")}</span>
        <span>{t("apiKeys.colRole")}</span>
        <span>{t("apiKeys.colScopes")}</span>
        <span>{t("apiKeys.colAssets")}</span>
        <span>{t("apiKeys.colCreated")}</span>
        <span>{t("apiKeys.colLastUsed")}</span>
        <span />
      </div>
    ),
    [t],
  );

  if (!isAdmin) {
    return (
      <Card className="mb-6">
        <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t("apiKeys.heading")}</p>
        <p className="text-xs text-[var(--muted)]">{t("apiKeys.adminRequired")}</p>
      </Card>
    );
  }

  return (
    <>
      <Card className="mb-6">
        <div className="flex items-center justify-between mb-1">
          <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">{t("apiKeys.heading")}</p>
        </div>
        <p className="text-xs text-[var(--muted)] mb-4">{t("apiKeys.description")}</p>

        {/* ── create form ── */}
        <div className="border border-[var(--line)] rounded-xl p-3 mb-4 space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-[1fr_8rem_8rem_auto] gap-2 items-end">
            <div>
              <label className="block text-[10px] uppercase tracking-wider text-[var(--muted)] mb-1">{t("apiKeys.name")}</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={120}
                placeholder={t("apiKeys.namePlaceholder")}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
              />
            </div>
            <div>
              <label className="block text-[10px] uppercase tracking-wider text-[var(--muted)] mb-1">{t("apiKeys.role")}</label>
              <select
                value={role}
                onChange={(e) => setRole(e.target.value)}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)] cursor-pointer"
              >
                <option value="admin">{t("apiKeys.roleAdmin")}</option>
                <option value="operator">{t("apiKeys.roleOperator")}</option>
                <option value="viewer">{t("apiKeys.roleViewer")}</option>
              </select>
            </div>
            <div>
              <label className="block text-[10px] uppercase tracking-wider text-[var(--muted)] mb-1">{t("apiKeys.expiresAt")}</label>
              <select
                value={expiry}
                onChange={(e) => setExpiry(e.target.value)}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-2.5 py-1.5 text-xs text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)] cursor-pointer"
              >
                <option value="30d">{t("apiKeys.expires30d")}</option>
                <option value="90d">{t("apiKeys.expires90d")}</option>
                <option value="1y">{t("apiKeys.expires1y")}</option>
                <option value="never">{t("apiKeys.expiresNever")}</option>
              </select>
            </div>
            <Button variant="primary" size="sm" loading={creating} disabled={!canCreate} onClick={() => { void handleCreate(); }}>
              <Plus size={13} />
              {t("apiKeys.createKey")}
            </Button>
          </div>

          {/* scope selector */}
          <div>
            <label className="block text-[10px] uppercase tracking-wider text-[var(--muted)] mb-1.5">{t("apiKeys.scopes")}</label>
            <ScopeSelector
              fullAccess={fullAccess}
              onFullAccessChange={setFullAccess}
              selectedScopes={selectedScopes}
              onToggleScope={handleToggleScope}
              onToggleGroup={handleToggleGroup}
            />
          </div>

          <AssetSelector
            assets={assets}
            inputName="api-key-create-assets-mode"
            restricted={restrictAssets}
            onRestrictedChange={setRestrictAssets}
            selectedAssetIDs={selectedAssetIDs}
            onToggleAsset={handleToggleAsset}
          />

          {createError && <p className="text-xs text-[var(--bad)]">{createError}</p>}
        </div>

        {/* ── loading / error / empty ── */}
        {loading && <p className="text-xs text-[var(--muted)] py-2">&nbsp;</p>}

        {!loading && error && <p className="text-xs text-[var(--bad)]">{error}</p>}

        {!loading && !error && keys.length === 0 && (
          <p className="text-xs text-[var(--muted)] py-1">{t("apiKeys.emptyState")}</p>
        )}

        {/* ── key table ── */}
        {!loading && keys.length > 0 && (
          <div className="overflow-x-auto rounded-xl border border-[var(--line)]">
            {tableHeader}
            {keys.map((k) => (
              <KeyRow
                key={k.id}
                keyInfo={k}
                assetNameByID={assetNameByID}
                onEdit={() => setDialog({ type: "edit", keyInfo: k })}
                onRevoke={() => setDialog({ type: "revoke", keyInfo: k })}
              />
            ))}
          </div>
        )}
      </Card>

      {/* ── key reveal modal ── */}
      {dialog?.type === "reveal" && (
        <KeyRevealModal
          created={dialog.created}
          onDismiss={() => setDialog(null)}
        />
      )}

      {/* ── revoke confirm modal ── */}
      {dialog?.type === "revoke" && (
        <RevokeConfirmModal
          keyInfo={dialog.keyInfo}
          onClose={() => setDialog(null)}
          onConfirm={() => handleRevoke(dialog.keyInfo)}
        />
      )}

      {dialog?.type === "edit" && (
        <EditKeyModal
          keyInfo={dialog.keyInfo}
          assets={assets}
          onClose={() => setDialog(null)}
          onConfirm={(request) => handleUpdate(dialog.keyInfo, request)}
        />
      )}
    </>
  );
}
