"use client";

import { Pencil, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ApiKeyInfo } from "../../../../hooks/useApiKeys";
import { formatWhen } from "./settingsModel";

/* ── one API key: name and prefix, role, what it reaches, last used, expiry, Edit and Revoke ── */

export type KeyRowProps = {
  keyInfo: ApiKeyInfo;
  assetNameByID: Map<string, string>;
  // False for another user's admin-role key when the caller is not the owner.
  canManage: boolean;
  onEdit: () => void;
  onRevoke: () => void;
};

export function KeyRow({ keyInfo, assetNameByID, canManage, onEdit, onRevoke }: KeyRowProps) {
  const t = useTranslations("settings");
  const now = new Date();
  const expired = Boolean(keyInfo.expires_at) && new Date(keyInfo.expires_at as string).getTime() <= now.getTime();
  const fullAccess = keyInfo.scopes.includes("*");
  const allowedAssets = keyInfo.allowed_assets ?? [];
  const access = allowedAssets.length === 0
    ? (fullAccess ? t("apiKeys.scopesFullAccess") : t("apiKeys.scopeCount", { count: keyInfo.scopes.length }))
    : t("apiKeys.assetCount", { count: allowedAssets.length });
  const accessTitle = [
    fullAccess ? t("apiKeys.scopesFullAccess") : keyInfo.scopes.join(", "),
    allowedAssets.length === 0 ? t("apiKeys.allowedAssetsAll") : allowedAssets.map((id) => assetNameByID.get(id) ?? id).join(", "),
  ].join(" · ");
  const roleKey = keyInfo.role === "admin" ? "roleAdmin" : keyInfo.role === "operator" ? "roleOperator" : keyInfo.role === "viewer" ? "roleViewer" : "";

  return (
    <div className="st-row st-tk">
      <div className="st-row-l" title={t("apiKeys.createdOn", { when: formatWhen(keyInfo.created_at, now) })}>
        <span className="st-nm">{keyInfo.name}</span>
        <code className="st-g st-mono">{keyInfo.prefix}…</code>
      </div>
      <span className="st-cell">{roleKey ? t(`apiKeys.${roleKey}`) : keyInfo.role}</span>
      <span className="st-cell st-hide-m" title={accessTitle}>{access}</span>
      <span className="st-cell st-num r c3 st-hide-m">{keyInfo.last_used_at ? formatWhen(keyInfo.last_used_at, now) : t("apiKeys.never")}</span>
      <span className="st-cell st-num r c3" title={keyInfo.expires_at ? formatWhen(keyInfo.expires_at, now) : undefined}>
        {!keyInfo.expires_at ? t("apiKeys.never") : expired ? <span className="st-bad">{t("apiKeys.expired")}</span> : formatWhen(keyInfo.expires_at, now)}
      </span>
      <div className="st-row-acts">
        {canManage ? (
          <>
            <button type="button" className="st-icon-btn" onClick={onEdit} aria-label={t("apiKeys.edit")}>
              <Pencil size={14} aria-hidden="true" />
            </button>
            <button type="button" className="st-icon-btn" onClick={onRevoke} aria-label={t("apiKeys.revoke")}>
              <Trash2 size={14} aria-hidden="true" />
            </button>
          </>
        ) : null}
      </div>
    </div>
  );
}
