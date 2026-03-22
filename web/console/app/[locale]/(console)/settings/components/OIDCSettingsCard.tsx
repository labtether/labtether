"use client";

import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { useOIDCSettings } from "../../../../hooks/useOIDCSettings";

type OIDCSettingsCardProps = {
  canManageUsers: boolean;
};

export function OIDCSettingsCard({ canManageUsers }: OIDCSettingsCardProps) {
  if (!canManageUsers) {
    return null;
  }

  return <OIDCSettingsCardInner />;
}

function OIDCSettingsCardInner() {
  const t = useTranslations("settings");
  const {
    enabled,
    setEnabled,
    issuerURL,
    setIssuerURL,
    clientID,
    setClientID,
    clientSecret,
    setClientSecret,
    scopes,
    setScopes,
    roleClaim,
    setRoleClaim,
    defaultRole,
    setDefaultRole,
    displayName,
    setDisplayName,
    adminRoleValues,
    setAdminRoleValues,
    operatorRoleValues,
    setOperatorRoleValues,
    autoProvision,
    setAutoProvision,
    sources,
    active,
    activeIssuer,
    loading,
    saving,
    applying,
    error,
    message,
    save,
    apply,
  } = useOIDCSettings();

  const isEnv = (key: string) => sources[key] === "env";

  return (
    <Card className="mb-6">
      <h2>{t("oidc.title")}</h2>
      <p className="text-sm text-[var(--muted)]">
        {t("oidc.description")}
      </p>

      <div className="mt-4 flex items-center gap-3">
        <span className="text-xs text-[var(--muted)]">{t("oidc.activeProvider")}</span>
        <span className="text-xs text-[var(--text)]">
          {active && activeIssuer ? activeIssuer : t("oidc.notActive")}
        </span>
      </div>

      <div className="mt-4 flex items-center gap-3">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
            disabled={loading || isEnv("enabled")}
            className="h-4 w-4 rounded border-[var(--line)] accent-[var(--accent)]"
          />
          <span className="text-sm text-[var(--text)]">{t("oidc.enableOIDC")}</span>
          {isEnv("enabled") ? <EnvBadge /> : null}
        </label>
      </div>

      {enabled ? (
        <div className="mt-4 grid gap-4 md:grid-cols-2">
          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.issuerURL")}
              {isEnv("issuer_url") ? <EnvBadge /> : null}
            </span>
            <Input
              value={issuerURL}
              onChange={(event) => setIssuerURL(event.target.value)}
              placeholder="https://accounts.example.com"
              disabled={loading || isEnv("issuer_url")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.clientID")}
              {isEnv("client_id") ? <EnvBadge /> : null}
            </span>
            <Input
              value={clientID}
              onChange={(event) => setClientID(event.target.value)}
              placeholder="my-client-id"
              disabled={loading || isEnv("client_id")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.clientSecret")}
              {isEnv("client_secret") ? <EnvBadge /> : null}
            </span>
            <Input
              type="password"
              value={clientSecret}
              onChange={(event) => setClientSecret(event.target.value)}
              placeholder={t("oidc.clientSecretPlaceholder")}
              disabled={loading || isEnv("client_secret")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.displayName")}
              {isEnv("display_name") ? <EnvBadge /> : null}
            </span>
            <Input
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              placeholder={t("oidc.displayNamePlaceholder")}
              disabled={loading || isEnv("display_name")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.scopes")}
              {isEnv("scopes") ? <EnvBadge /> : null}
            </span>
            <Input
              value={scopes}
              onChange={(event) => setScopes(event.target.value)}
              placeholder="openid,profile,email"
              disabled={loading || isEnv("scopes")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.roleClaim")}
              {isEnv("role_claim") ? <EnvBadge /> : null}
            </span>
            <Input
              value={roleClaim}
              onChange={(event) => setRoleClaim(event.target.value)}
              placeholder="groups"
              disabled={loading || isEnv("role_claim")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.defaultRole")}
              {isEnv("default_role") ? <EnvBadge /> : null}
            </span>
            <Select
              value={defaultRole}
              onChange={(event) => setDefaultRole(event.target.value)}
              disabled={loading || isEnv("default_role")}
            >
              <option value="viewer">{t("oidc.roleViewer")}</option>
              <option value="operator">{t("oidc.roleOperator")}</option>
              <option value="admin">{t("oidc.roleAdmin")}</option>
            </Select>
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.adminRoleValues")}
              {isEnv("admin_role_values") ? <EnvBadge /> : null}
            </span>
            <Input
              value={adminRoleValues}
              onChange={(event) => setAdminRoleValues(event.target.value)}
              placeholder="labtether-admin"
              disabled={loading || isEnv("admin_role_values")}
            />
          </label>

          <label className="text-xs text-[var(--muted)] flex flex-col gap-1.5">
            <span className="flex items-center gap-1.5">
              {t("oidc.operatorRoleValues")}
              {isEnv("operator_role_values") ? <EnvBadge /> : null}
            </span>
            <Input
              value={operatorRoleValues}
              onChange={(event) => setOperatorRoleValues(event.target.value)}
              placeholder="labtether-operator"
              disabled={loading || isEnv("operator_role_values")}
            />
          </label>

          <div className="flex items-center gap-3 md:col-span-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={autoProvision}
                onChange={(event) => setAutoProvision(event.target.checked)}
                disabled={loading || isEnv("auto_provision")}
                className="h-4 w-4 rounded border-[var(--line)] accent-[var(--accent)]"
              />
              <span className="text-sm text-[var(--text)]">{t("oidc.autoProvision")}</span>
              {isEnv("auto_provision") ? <EnvBadge /> : null}
            </label>
          </div>
        </div>
      ) : null}

      {error ? <p className="mt-3 text-sm text-[var(--bad)]">{error}</p> : null}
      {message ? <p className="mt-3 text-sm text-[var(--muted)]">{message}</p> : null}

      <div className="mt-4 flex items-center gap-2">
        <Button variant="primary" loading={saving} disabled={loading || applying} onClick={() => void save()}>
          {t("oidc.save")}
        </Button>
        <Button variant="secondary" loading={applying} disabled={loading || saving} onClick={() => void apply()}>
          {t("oidc.apply")}
        </Button>
      </div>
    </Card>
  );
}

function EnvBadge() {
  return (
    <span className="text-[10px] px-1.5 py-0.5 rounded border border-[var(--accent)]/40 text-[var(--accent)] font-mono">
      ENV
    </span>
  );
}
