"use client";

import { Pencil, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ApiKeyInfo } from "../../../../hooks/useApiKeys";
import { relativeTime } from "./apiKeyPresentation";

/* ── key table row ── */

export type KeyRowProps = {
  keyInfo: ApiKeyInfo;
  assetNameByID: Map<string, string>;
  onEdit: () => void;
  onRevoke: () => void;
};

export function KeyRow({ keyInfo, assetNameByID, onEdit, onRevoke }: KeyRowProps) {
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
