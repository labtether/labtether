"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { useOIDCSettings } from "../../../../hooks/useOIDCSettings";
import { Field, Row, Section, Sheet, SheetFoot, Switch } from "./SettingsKit";

type OIDCSettingsCardProps = {
  canManageUsers: boolean;
  // Only the owner may save SSO settings; the Hub refuses everyone else.
  canEdit: boolean;
};

export function OIDCSettingsCard({ canManageUsers, canEdit }: OIDCSettingsCardProps) {
  if (!canManageUsers) {
    return null;
  }

  return <OIDCSettingsCardInner canEdit={canEdit} />;
}

function OIDCSettingsCardInner({ canEdit }: { canEdit: boolean }) {
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
    clientSecretConfigured,
    clearClientSecret,
    removeClientSecret,
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
  const envTag = (key: string) => (isEnv(key) ? <span className="st-tag">{t("oidc.fromEnv")}</span> : null);
  const text = (key: string, id: string, label: string, value: string, set: (value: string) => void, placeholder?: string) => (
    <Field label={<>{label}{envTag(key)}</>} htmlFor={id}>
      <Input id={id} value={value} onChange={(event) => set(event.target.value)} placeholder={placeholder} disabled={loading || isEnv(key)} />
    </Field>
  );
  const footMessage = error || message || (canEdit ? "" : t("oidc.ownerOnly"));

  return (
    <Section title={t("oidc.title")} id="sso">
      <Sheet>
        <fieldset disabled={!canEdit} className="st-fieldset">
          <Row label={t("oidc.enableOIDC")} hint={active && activeIssuer ? t("oidc.activeIssuer", { issuer: activeIssuer }) : t("oidc.notActive")} className="st-sw-row">
            {envTag("enabled")}
            <Switch checked={enabled} onChange={setEnabled} label={t("oidc.enableOIDC")} disabled={loading || isEnv("enabled")} />
          </Row>

          {enabled ? (
            <div className="st-form">
              {text("issuer_url", "st-oidc-issuer", t("oidc.issuerURL"), issuerURL, setIssuerURL, "https://auth.example.com")}
              {text("client_id", "st-oidc-client", t("oidc.clientID"), clientID, setClientID, "labtether")}
              <Field
                label={<>{t("oidc.clientSecret")}{envTag("client_secret")}</>}
                htmlFor="oidc-client-secret"
                hint={clearClientSecret
                  ? <span className="st-g bad">{t("oidc.clientSecretWillBeRemoved")}</span>
                  : !isEnv("client_secret") && clientSecretConfigured
                    ? (
                      <span className="st-cv">
                        {t("oidc.clientSecretConfigured")}
                        <button type="button" className="st-link" onClick={removeClientSecret}>{t("oidc.removeClientSecret")}</button>
                      </span>
                    )
                    : null}
              >
                <Input
                  id="oidc-client-secret"
                  type="password"
                  value={clientSecret}
                  onChange={(event) => setClientSecret(event.target.value)}
                  placeholder={t("oidc.clientSecretPlaceholder")}
                  disabled={loading || isEnv("client_secret")}
                  autoComplete="new-password"
                />
              </Field>
              {text("display_name", "st-oidc-name", t("oidc.displayName"), displayName, setDisplayName, t("oidc.displayNamePlaceholder"))}
              {text("scopes", "st-oidc-scopes", t("oidc.scopes"), scopes, setScopes, "openid,profile,email")}
              {text("role_claim", "st-oidc-claim", t("oidc.roleClaim"), roleClaim, setRoleClaim, "groups")}
              <Field label={<>{t("oidc.defaultRole")}{envTag("default_role")}</>} htmlFor="st-oidc-role">
                <Select id="st-oidc-role" className="w-full" value={defaultRole} onChange={(event) => setDefaultRole(event.target.value)} disabled={loading || isEnv("default_role")}>
                  <option value="viewer">{t("oidc.roleViewer")}</option>
                  <option value="operator">{t("oidc.roleOperator")}</option>
                  <option value="admin">{t("oidc.roleAdmin")}</option>
                </Select>
              </Field>
              {text("admin_role_values", "st-oidc-admins", t("oidc.adminRoleValues"), adminRoleValues, setAdminRoleValues, "labtether-admin")}
              {text("operator_role_values", "st-oidc-operators", t("oidc.operatorRoleValues"), operatorRoleValues, setOperatorRoleValues, "labtether-operator")}
              <label className="st-check wide">
                <input
                  type="checkbox"
                  checked={autoProvision}
                  onChange={(event) => setAutoProvision(event.target.checked)}
                  disabled={loading || isEnv("auto_provision")}
                />
                {t("oidc.autoProvision")}
                {envTag("auto_provision")}
              </label>
            </div>
          ) : null}
        </fieldset>

        <SheetFoot message={footMessage || null} tone={error ? "bad" : undefined}>
          {canEdit ? (
            <Button size="sm" loading={saving} disabled={loading || applying} onClick={() => void save()}>
              {t("oidc.save")}
            </Button>
          ) : null}
          <Button size="sm" variant="ghost" loading={applying} disabled={loading || saving} onClick={() => void apply()}>
            {t("oidc.apply")}
          </Button>
        </SheetFoot>
      </Sheet>
    </Section>
  );
}
