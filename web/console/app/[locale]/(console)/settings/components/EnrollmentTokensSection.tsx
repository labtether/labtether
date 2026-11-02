"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { EnrollmentToken } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import type { EnrollmentTokenScopeRequest } from "../../../../hooks/useEnrollment";
import { RowMenu } from "./RowMenu";
import { CodeBlock, CopyButton, Field, Section, Sheet, Status } from "./SettingsKit";
import { enrollmentTokenState, formatWhen } from "./settingsModel";

type Scope = EnrollmentTokenScopeRequest["scope"];

type EnrollmentTokensSectionProps = {
  enrollmentTokens: EnrollmentToken[];
  wsURL: string;
  newRawToken: string;
  generating: boolean;
  generateToken: (label: string, ttlHours: number, maxUses: number, scope: EnrollmentTokenScopeRequest) => Promise<unknown> | void;
  revokeEnrollmentToken: (id: string) => Promise<void> | void;
  clearNewToken: () => void;
  /** The username for a user ID, when the hub's user list has it. */
  userName: (id: string) => string | undefined;
};

/** Enrollment tokens: Create token opens the form, a new token's value shows once, then the list with Disable. */
export function EnrollmentTokensSection({
  enrollmentTokens,
  wsURL,
  newRawToken,
  generating,
  generateToken,
  revokeEnrollmentToken,
  clearNewToken,
  userName,
}: EnrollmentTokensSectionProps) {
  const t = useTranslations("settings");
  const locale = useLocale();
  const [formOpen, setFormOpen] = useState(false);

  // A user ID is not shown: "by alex" only when the user list names the creator.
  const creator = (token: EnrollmentToken) => (token.created_by ? userName(token.created_by) : undefined);
  const scopeText = (token: EnrollmentToken) => {
    if (token.scope === "asset") return t("agents.forDevice", { id: token.asset_id ?? "" });
    if (token.scope === "group") return t("agents.forGroup", { id: token.allowed_group_id ?? "" });
    if (token.scope === "unplaced") return t("agents.forUnplaced");
    if (token.scope === "unrestricted") return t("agents.forUnrestricted");
    return token.scope.replaceAll("_", " ");
  };

  return (
    <Section
      title={t("agents.enrollmentTokens")}
      id="enrollment"
      action={(
        <Button size="sm" onClick={() => { clearNewToken(); setFormOpen((open) => !open); }} aria-expanded={formOpen}>
          {t("agents.createToken")}
        </Button>
      )}
    >
      {formOpen ? (
        <TokenForm
          generating={generating}
          onCancel={() => setFormOpen(false)}
          onCreate={async (label, ttl, uses, scope) => {
            // The hook returns the new token's id, or "" when the hub refused (its error shows above the page).
            const created = await generateToken(label, ttl, uses, scope);
            if (created) setFormOpen(false);
          }}
        />
      ) : null}

      {newRawToken ? (
        <Sheet>
          <div className="st-once">
            <p className="st-nm">{t("agents.newToken")}</p>
            <div className="st-once-r">
              <code className="st-secret" aria-label={t("agents.newToken")}>{newRawToken}</code>
              <CopyButton text={newRawToken} label={t("agents.copyToken")} copiedLabel={t("agents.copied")} />
              <Button size="sm" variant="ghost" onClick={clearNewToken}>{t("agents.dismiss")}</Button>
            </div>
            <p className="st-g">{t("agents.shownOnce")}</p>
            <details className="st-details">
              <summary className="st-link">{t("agents.copyPasteSetup")}</summary>
              <CodeBlock
                copyLabel={t("agents.copySetup")}
                code={`# Docker
docker run -e LABTETHER_ENROLLMENT_TOKEN="${newRawToken}" \\
  -e LABTETHER_WS_URL="${wsURL}" \\
  labtether/labtether-agent

# Linux
export LABTETHER_ENROLLMENT_TOKEN="${newRawToken}"
export LABTETHER_WS_URL="${wsURL}"
./labtether-agent`}
              />
            </details>
          </div>
        </Sheet>
      ) : null}

      <Sheet>
        {enrollmentTokens.length > 0 ? (
          <div className="st-row st-en st-cols" aria-hidden="true">
            <span>{t("agents.colToken")}</span>
            <span className="st-hide-m">{t("agents.colUsed")}</span>
            <span className="r">{t("agents.colExpires")}</span>
            <span>{t("agents.colState")}</span>
            <span />
          </div>
        ) : null}
        {enrollmentTokens.map((token) => {
          const state = enrollmentTokenState(token);
          const name = token.label || t("agents.unnamedToken");
          return (
            <div key={token.id} className="st-row st-en">
              <div className="st-row-l">
                <span className="st-nm">{name}</span>
                <span className="st-g">{creator(token) ? `${scopeText(token)} · ${t("agents.byUser", { user: creator(token) as string })}` : scopeText(token)}</span>
              </div>
              <span className="st-cell st-num st-hide-m">
                {token.max_uses > 0 ? t("agents.used", { used: token.use_count, max: token.max_uses }) : t("agents.usedNoLimit", { used: token.use_count })}
              </span>
              <span className="st-cell st-num r c3">{formatWhen(token.expires_at, new Date(), locale)}</span>
              <span className="st-cell st-keep">
                {state === "active" ? t("agents.state.active") : <Status tone="off">{t(`agents.state.${state}`)}</Status>}
              </span>
              <RowMenu
                label={t("agents.actionsFor", { name })}
                items={state === "revoked" ? [] : [{ label: t("agents.disable"), danger: true, onSelect: () => void revokeEnrollmentToken(token.id) }]}
              />
            </div>
          );
        })}
        {enrollmentTokens.length === 0 ? <p className="st-msg st-pad-row">{t("agents.noTokens")}</p> : null}
      </Sheet>
    </Section>
  );
}

function TokenForm({
  generating,
  onCancel,
  onCreate,
}: {
  generating: boolean;
  onCancel: () => void;
  onCreate: (label: string, ttlHours: number, maxUses: number, scope: EnrollmentTokenScopeRequest) => Promise<void>;
}) {
  const t = useTranslations("settings");
  const [label, setLabel] = useState("");
  const [ttl, setTTL] = useState(24);
  const [maxUses, setMaxUses] = useState(1);
  const [scope, setScope] = useState<Scope>("asset");
  const [assetID, setAssetID] = useState("");
  const [groupID, setGroupID] = useState("");
  const [acknowledge, setAcknowledge] = useState(false);
  const ready =
    (scope === "asset" && assetID.trim() !== "") ||
    (scope === "group" && groupID.trim() !== "") ||
    scope === "unplaced" ||
    (scope === "unrestricted" && acknowledge);

  return (
    <Sheet pad>
      <form
        className="st-form three"
        aria-label={t("agents.createToken")}
        onSubmit={(event) => {
          event.preventDefault();
          if (!ready || generating) return;
          void onCreate(label, ttl, maxUses, {
            scope,
            assetID: scope === "asset" ? assetID : undefined,
            allowedGroupID: scope === "group" ? groupID : undefined,
            acknowledgeUnrestricted: scope === "unrestricted" ? acknowledge : undefined,
          });
        }}
      >
        <Field label={t("agents.scope")} htmlFor="st-en-scope">
          <Select
            id="st-en-scope"
            className="w-full"
            value={scope}
            onChange={(event) => {
              const next = event.target.value as Scope;
              setScope(next);
              setAcknowledge(false);
              if (next === "asset") setMaxUses(1);
            }}
          >
            <option value="asset">{t("agents.scopeAsset")}</option>
            <option value="group">{t("agents.scopeGroup")}</option>
            <option value="unplaced">{t("agents.scopeUnplaced")}</option>
            <option value="unrestricted">{t("agents.scopeUnrestricted")}</option>
          </Select>
        </Field>
        {scope === "asset" ? (
          <Field label={t("agents.hostname")} htmlFor="st-en-host">
            <Input id="st-en-host" value={assetID} placeholder="nas01" onChange={(event) => setAssetID(event.target.value)} autoComplete="off" />
          </Field>
        ) : null}
        {scope === "group" ? (
          <Field label={t("agents.groupId")} htmlFor="st-en-group">
            <Input id="st-en-group" value={groupID} onChange={(event) => setGroupID(event.target.value)} autoComplete="off" />
          </Field>
        ) : null}
        <Field label={t("agents.label")} htmlFor="st-en-label">
          <Input id="st-en-label" value={label} placeholder={t("agents.labelPlaceholder")} onChange={(event) => setLabel(event.target.value)} autoComplete="off" />
        </Field>
        <Field label={t("agents.validForHours")} htmlFor="st-en-ttl">
          <Input id="st-en-ttl" type="number" min={1} max={720} value={ttl} onChange={(event) => setTTL(Number(event.target.value))} />
        </Field>
        <Field label={t("agents.maxUses")} htmlFor="st-en-uses">
          <Input id="st-en-uses" type="number" min={1} disabled={scope === "asset"} value={maxUses} onChange={(event) => setMaxUses(Number(event.target.value))} />
        </Field>
        {scope === "unrestricted" ? (
          <label className="st-check bad wide">
            <input type="checkbox" checked={acknowledge} onChange={(event) => setAcknowledge(event.target.checked)} />
            {t("agents.ackUnrestricted")}
          </label>
        ) : null}
        <div className="st-acts st-field wide">
          <Button type="submit" variant="primary" disabled={generating || !ready}>
            {generating ? t("agents.generating") : t("agents.create")}
          </Button>
          <Button type="button" variant="ghost" onClick={onCancel}>{t("agents.cancel")}</Button>
        </div>
      </form>
    </Sheet>
  );
}
