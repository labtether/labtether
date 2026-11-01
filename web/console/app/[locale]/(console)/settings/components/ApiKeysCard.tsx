"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { useAuth } from "../../../../contexts/AuthContext";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useApiKeys } from "../../../../hooks/useApiKeys";
import type { ApiKeyInfo, CreateKeyRequest, CreatedKeyResponse, UpdateKeyRequest } from "../../../../hooks/useApiKeys";
import { canManageApiKey } from "../../../../lib/roles";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";
import { type AssetOption, ALL_SCOPES, ScopeSelector, AssetSelector } from "./ApiKeyAccessSelectors";
import { expiryToIso } from "./apiKeyPresentation";
import { KeyRow } from "./ApiKeyRow";
import { KeyRevealModal, RevokeConfirmModal } from "./ApiKeySecurityDialogs";
import { EditKeyModal } from "./EditKeyModal";
import { Field, Message, Section, Sheet } from "./SettingsKit";

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
  const [formOpen, setFormOpen] = useState(false);

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
      setFormOpen(false);
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

  if (!isAdmin) {
    return (
      <Section title={t("apiKeys.heading")} id="api-tokens">
        <Sheet pad>
          <p className="st-msg">{t("apiKeys.adminRequired")}</p>
        </Sheet>
      </Section>
    );
  }

  return (
    <Section
      title={t("apiKeys.heading")}
      id="api-tokens"
      action={(
        <Button size="sm" onClick={() => setFormOpen((open) => !open)} aria-expanded={formOpen}>
          {t("apiKeys.createKey")}
        </Button>
      )}
    >
      {formOpen ? (
        <Sheet pad>
          <form
            className="st-form three"
            aria-label={t("apiKeys.createKey")}
            onSubmit={(event) => {
              event.preventDefault();
              void handleCreate();
            }}
          >
            <Field label={t("apiKeys.name")} htmlFor="st-key-name" wide>
              <Input
                id="st-key-name"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={120}
                placeholder={t("apiKeys.namePlaceholder")}
                autoComplete="off"
              />
            </Field>
            <Field label={t("apiKeys.role")} htmlFor="st-key-role">
              <Select id="st-key-role" className="w-full" value={role} onChange={(e) => setRole(e.target.value)}>
                <option value="admin">{t("apiKeys.roleAdmin")}</option>
                <option value="operator">{t("apiKeys.roleOperator")}</option>
                <option value="viewer">{t("apiKeys.roleViewer")}</option>
              </Select>
            </Field>
            <Field label={t("apiKeys.expiresAt")} htmlFor="st-key-expiry">
              <Select id="st-key-expiry" className="w-full" value={expiry} onChange={(e) => setExpiry(e.target.value)}>
                <option value="30d">{t("apiKeys.expires30d")}</option>
                <option value="90d">{t("apiKeys.expires90d")}</option>
                <option value="1y">{t("apiKeys.expires1y")}</option>
                <option value="never">{t("apiKeys.expiresNever")}</option>
              </Select>
            </Field>
            <div className="st-field wide">
              <span className="st-lbl">{t("apiKeys.scopes")}</span>
              <ScopeSelector
                fullAccess={fullAccess}
                onFullAccessChange={setFullAccess}
                selectedScopes={selectedScopes}
                onToggleScope={handleToggleScope}
                onToggleGroup={handleToggleGroup}
              />
            </div>
            <div className="st-field wide">
              <AssetSelector
                assets={assets}
                inputName="api-key-create-assets-mode"
                restricted={restrictAssets}
                onRestrictedChange={setRestrictAssets}
                selectedAssetIDs={selectedAssetIDs}
                onToggleAsset={handleToggleAsset}
              />
            </div>
            {createError ? <p className="st-msg bad st-field wide" role="alert">{createError}</p> : null}
            <div className="st-acts st-field wide">
              <Button type="submit" variant="primary" loading={creating} disabled={!canCreate}>
                {t("apiKeys.create")}
              </Button>
              <Button type="button" variant="ghost" onClick={() => setFormOpen(false)}>{t("apiKeys.cancel")}</Button>
            </div>
          </form>
        </Sheet>
      ) : null}

      <Sheet className={loading && keys.length === 0 ? "st-loading" : ""}>
        {keys.length > 0 ? (
          <div className="st-row st-tk st-cols" aria-hidden="true">
            <span>{t("apiKeys.colName")}</span>
            <span>{t("apiKeys.colRole")}</span>
            <span className="st-hide-m">{t("apiKeys.colAccess")}</span>
            <span className="r st-hide-m">{t("apiKeys.colLastUsed")}</span>
            <span className="r">{t("apiKeys.expiresAt")}</span>
            <span />
          </div>
        ) : null}
        {keys.map((k) => (
          <KeyRow
            key={k.id}
            keyInfo={k}
            assetNameByID={assetNameByID}
            canManage={canManageApiKey(user, k)}
            onEdit={() => setDialog({ type: "edit", keyInfo: k })}
            onRevoke={() => setDialog({ type: "revoke", keyInfo: k })}
          />
        ))}
        {!loading && !error && keys.length === 0 ? (
          <div className="st-empty">
            <b>{t("apiKeys.emptyState")}</b>
          </div>
        ) : null}
        {!loading && error ? <Message tone="bad">{error}</Message> : null}
      </Sheet>

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
    </Section>
  );
}
