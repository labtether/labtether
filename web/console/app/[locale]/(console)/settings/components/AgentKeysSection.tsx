"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { AgentTokenSummary, EnrollmentToken } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import type { AgentTokenRevokeResult } from "../../../../hooks/useEnrollment";
import { RowMenu } from "./RowMenu";
import { Message, Section, SettingsDialog, Sheet, Status } from "./SettingsKit";
import { deadTokenCount, formatWhen } from "./settingsModel";

type AgentKeysSectionProps = {
  enrollmentTokens: EnrollmentToken[];
  agentTokens: AgentTokenSummary[];
  revokeAgentToken: (id: string) => Promise<AgentTokenRevokeResult | null>;
  cleanupDeadTokens: () => Promise<{ enrollment_deleted: number; agent_deleted: number } | null>;
};

/** The key each enrolled agent signs in with: device, fingerprint, last sign-in; Disable cuts that agent off. */
export function AgentKeysSection({ enrollmentTokens, agentTokens, revokeAgentToken, cleanupDeadTokens }: AgentKeysSectionProps) {
  const t = useTranslations("settings");
  const locale = useLocale();
  const [revokeNotice, setRevokeNotice] = useState("");
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [cleaning, setCleaning] = useState(false);
  const [cleanupResult, setCleanupResult] = useState("");
  const totalDead = deadTokenCount(enrollmentTokens, agentTokens);

  const handleRevoke = async (token: AgentTokenSummary) => {
    setRevokeNotice("");
    const result = await revokeAgentToken(token.id);
    if (result?.hubSSHKey === "not_removed") {
      setRevokeNotice(t("agents.hubSSHKeyNotRemoved", { device: token.asset_id }));
    }
  };

  const handleCleanup = async () => {
    setCleaning(true);
    setCleanupResult("");
    const result = await cleanupDeadTokens();
    setCleaning(false);
    setCleanupOpen(false);
    if (result) {
      setCleanupResult(t("agents.cleared", { count: result.enrollment_deleted + result.agent_deleted }));
    }
  };

  return (
    <Section
      title={t("agents.connectedAgents")}
      id="agent-keys"
      action={totalDead > 0 ? (
        <Button size="sm" variant="ghost" disabled={cleaning} onClick={() => setCleanupOpen(true)}>
          {cleaning ? t("agents.clearing") : t("agents.clearDeadTokens", { count: totalDead })}
        </Button>
      ) : null}
    >
      <Sheet>
        {agentTokens.length > 0 ? (
          <div className="st-row st-ak st-cols" aria-hidden="true">
            <span>{t("agents.colDevice")}</span>
            <span className="st-hide-m">{t("agents.fingerprint")}</span>
            <span className="r">{t("agents.colLastAuth")}</span>
            <span>{t("agents.colState")}</span>
            <span />
          </div>
        ) : null}
        {agentTokens.map((token) => (
          <div key={token.id} className="st-row st-ak">
            <div className="st-row-l">
              <span className="st-nm">{token.asset_id}</span>
              <span className="st-g">{t("agents.added", { when: formatWhen(token.created_at, new Date(), locale) })}</span>
            </div>
            <code className="st-cell st-mono st-hide-m">{token.device_fingerprint || "—"}</code>
            <span className="st-cell st-num r c3">{formatWhen(token.last_used_at, new Date(), locale) || "—"}</span>
            <span className="st-cell st-keep">
              {token.status === "active" ? t("agents.state.active") : <Status tone="off">{t("agents.state.revoked")}</Status>}
            </span>
            <RowMenu
              label={t("agents.actionsFor", { name: token.asset_id })}
              items={token.status === "active" ? [{ label: t("agents.disable"), danger: true, onSelect: () => void handleRevoke(token) }] : []}
            />
          </div>
        ))}
        {agentTokens.length === 0 ? <p className="st-msg st-pad-row">{t("agents.noKeys")}</p> : null}
        {revokeNotice ? <Message tone="bad">{revokeNotice}</Message> : null}
        {cleanupResult ? <Message>{cleanupResult}</Message> : null}
      </Sheet>

      {cleanupOpen ? (
        <SettingsDialog labelledBy="st-cleanup-title" onClose={() => { if (!cleaning) setCleanupOpen(false); }}>
          <h3 id="st-cleanup-title">{t("agents.deleteCount", { count: totalDead })}</h3>
          <p>{t("agents.deleteBody")}</p>
          <div className="st-dlg-acts">
            <Button onClick={() => setCleanupOpen(false)} disabled={cleaning}>{t("agents.cancel")}</Button>
            <Button variant="danger" className="st-danger-fill" loading={cleaning} onClick={() => void handleCleanup()}>
              {t("agents.deleteConfirm")}
            </Button>
          </div>
        </SettingsDialog>
      ) : null}
    </Section>
  );
}
