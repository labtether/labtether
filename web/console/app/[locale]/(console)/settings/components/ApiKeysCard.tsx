"use client";

import { useCallback, useMemo, useState } from "react";
import { Check, Copy, Key, Pencil, Plus, ShieldAlert, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { useAuth } from "../../../../contexts/AuthContext";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useApiKeys } from "../../../../hooks/useApiKeys";
import type { ApiKeyInfo, CreateKeyRequest, CreatedKeyResponse, UpdateKeyRequest } from "../../../../hooks/useApiKeys";
import { sanitizeErrorMessage } from "../../../../lib/sanitizeErrorMessage";

/* ── scope categories (mirrors internal/apikeys/scope.go knownScopeCategories) ── */

const SCOPE_GROUPS: { label: string; scopes: string[] }[] = [
  { label: "Assets & Inventory", scopes: ["assets", "groups", "topology", "discovery"] },
  { label: "Operations", scopes: ["shell", "files", "services", "processes", "cron"] },
  { label: "Monitoring", scopes: ["alerts", "metrics", "logs", "incidents", "notifications"] },
  { label: "System", scopes: ["network", "disks", "packages", "users", "settings", "updates"] },
  { label: "Integrations", scopes: ["docker", "connectors", "homeassistant", "agents", "collectors", "web-services"] },
  { label: "Automation", scopes: ["webhooks", "schedules", "actions", "events", "bulk"] },
  { label: "Platform", scopes: ["hub", "failover", "terminal", "search", "dead-letters", "credentials", "audit"] },
];

const ALL_SCOPES = SCOPE_GROUPS.flatMap((g) => g.scopes);

/* ── helpers ── */

function expiryToIso(value: string): string | null {
  if (value === "never") return null;
  const now = Date.now();
  const msPerDay = 86_400_000;
  switch (value) {
    case "30d":
      return new Date(now + 30 * msPerDay).toISOString();
    case "90d":
      return new Date(now + 90 * msPerDay).toISOString();
    case "1y":
      return new Date(now + 365 * msPerDay).toISOString();
    default:
      return null;
  }
}

function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "Never";
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return "just now";
  const mins = Math.floor(diff / 60_000);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

/* ── scope selector ── */

type ScopeSelectorProps = {
  fullAccess: boolean;
  onFullAccessChange: (v: boolean) => void;
  selectedScopes: Set<string>;
  onToggleScope: (scope: string) => void;
  onToggleGroup: (scopes: string[], allSelected: boolean) => void;
};

function ScopeSelector({ fullAccess, onFullAccessChange, selectedScopes, onToggleScope, onToggleGroup }: ScopeSelectorProps) {
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

type AssetOption = {
  id: string;
  name: string;
};

type AssetSelectorProps = {
  assets: AssetOption[];
  inputName: string;
  restricted: boolean;
  onRestrictedChange: (value: boolean) => void;
  selectedAssetIDs: Set<string>;
  onToggleAsset: (assetID: string) => void;
};

function AssetSelector({
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

/* ── key reveal modal ── */

type KeyRevealModalProps = {
  created: CreatedKeyResponse;
  onDismiss: () => void;
};

function KeyRevealModal({ created, onDismiss }: KeyRevealModalProps) {
  const t = useTranslations("settings");
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(created.raw_key);
      setCopied(true);
      setTimeout(() => setCopied(false), 3000);
    } catch {
      // fallback: do nothing
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="api-key-reveal-title"
        onClick={(e) => e.stopPropagation()}
      >
        <Card className="w-[32rem] max-w-[92vw] space-y-4">
          <div className="flex items-center gap-2">
            <Key size={16} className="text-[var(--accent)]" />
            <h3 id="api-key-reveal-title" className="text-sm font-medium text-[var(--text)]">{t("apiKeys.revealTitle")}</h3>
          </div>
          <div className="bg-[var(--surface)] rounded-lg p-3 font-mono text-xs text-[var(--text)] break-all select-all">
            {created.raw_key}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => { void handleCopy(); }}>
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? t("apiKeys.revealCopied") : "Copy"}
            </Button>
          </div>
          <p className="text-xs text-[var(--bad)] flex items-center gap-1.5">
            <ShieldAlert size={13} className="shrink-0" />
            {t("apiKeys.revealWarning")}
          </p>
          <div className="flex justify-end">
            <Button variant="primary" size="sm" onClick={onDismiss}>
              {t("apiKeys.revealDismiss")}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

/* ── revoke confirm modal ── */

type RevokeConfirmModalProps = {
  keyInfo: ApiKeyInfo;
  onClose: () => void;
  onConfirm: () => Promise<void>;
};

function RevokeConfirmModal({ keyInfo, onClose, onConfirm }: RevokeConfirmModalProps) {
  const t = useTranslations("settings");
  const [revoking, setRevoking] = useState(false);
  const [error, setError] = useState("");

  const handleConfirm = async () => {
    setRevoking(true);
    setError("");
    try {
      await onConfirm();
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "Failed to revoke key."));
      setRevoking(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={() => { if (!revoking) onClose(); }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="api-key-revoke-title"
        onClick={(e) => e.stopPropagation()}
      >
        <Card className="w-[28rem] max-w-[92vw] space-y-4">
          <h3 id="api-key-revoke-title" className="text-sm font-medium text-[var(--text)]">{t("apiKeys.revokeTitle")}</h3>
          <p className="text-xs text-[var(--muted)]">
            {t("apiKeys.revokeBody", { name: keyInfo.name })}
          </p>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={revoking}>{t("apiKeys.cancel")}</Button>
            <Button variant="danger" loading={revoking} onClick={() => { void handleConfirm(); }}>{t("apiKeys.revokeConfirm")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

type EditKeyModalProps = {
  keyInfo: ApiKeyInfo;
  assets: AssetOption[];
  onClose: () => void;
  onConfirm: (request: UpdateKeyRequest) => Promise<void>;
};

function EditKeyModal({ keyInfo, assets, onClose, onConfirm }: EditKeyModalProps) {
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

/* ── key table row ── */

type KeyRowProps = {
  keyInfo: ApiKeyInfo;
  assetNameByID: Map<string, string>;
  onEdit: () => void;
  onRevoke: () => void;
};

function KeyRow({ keyInfo, assetNameByID, onEdit, onRevoke }: KeyRowProps) {
  const t = useTranslations("settings");
  const scopeLabel = keyInfo.scopes.includes("*")
    ? t("apiKeys.scopesFullAccess")
    : keyInfo.scopes.length > 3
      ? `${keyInfo.scopes.slice(0, 3).join(", ")} +${keyInfo.scopes.length - 3}`
      : keyInfo.scopes.join(", ");
  const allowedAssets = keyInfo.allowed_assets ?? [];
  const assetLabel = allowedAssets.length === 0
    ? t("apiKeys.allowedAssetsAll")
    : t("apiKeys.assetCount", { count: allowedAssets.length });
  const assetTitle = allowedAssets.length === 0
    ? t("apiKeys.allowedAssetsAll")
    : allowedAssets.map((id) => assetNameByID.get(id) ?? id).join(", ");

  return (
    <div className="grid min-w-[58rem] grid-cols-[5rem_1fr_4rem_1fr_8rem_5.5rem_5.5rem_4rem] items-center gap-2 border-t border-[var(--line)] px-3 py-2 text-xs">
      <span className="font-mono text-[var(--muted)] truncate">{keyInfo.prefix}...</span>
      <span className="text-[var(--text)] truncate">{keyInfo.name}</span>
      <span className="text-[var(--muted)]">{keyInfo.role}</span>
      <span className="text-[var(--muted)] truncate" title={keyInfo.scopes.join(", ")}>{scopeLabel}</span>
      <span className="truncate text-[var(--muted)]" title={assetTitle}>{assetLabel}</span>
      <span className="text-[var(--muted)]">{relativeTime(keyInfo.created_at)}</span>
      <span className="text-[var(--muted)]">{keyInfo.last_used_at ? relativeTime(keyInfo.last_used_at) : t("apiKeys.never")}</span>
      <div className="flex justify-end gap-1">
        <button
          onClick={onEdit}
          className="flex h-6 w-6 cursor-pointer items-center justify-center rounded-md border-none bg-transparent text-[var(--muted)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--text)]"
          aria-label={t("apiKeys.edit")}
          title={t("apiKeys.edit")}
        >
          <Pencil size={13} />
        </button>
        <button
          onClick={onRevoke}
          className="flex items-center justify-center h-6 w-6 rounded-md text-[var(--bad)] hover:bg-[var(--bad)]/10 transition-colors cursor-pointer bg-transparent border-none"
          aria-label={t("apiKeys.revoke")}
          title={t("apiKeys.revoke")}
        >
          <Trash2 size={13} />
        </button>
      </div>
    </div>
  );
}

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
