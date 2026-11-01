"use client";

import { useTranslations } from "next-intl";
import { PageHeader } from "../../../../components/PageHeader";
import { useAuth } from "../../../../contexts/AuthContext";
import { useEnrollment } from "../../../../hooks/useEnrollment";
import { hasAdminRole } from "../../../../lib/roles";
import { AgentConnectionSection } from "../components/AgentConnectionSection";
import { AgentKeysSection } from "../components/AgentKeysSection";
import { EnrollmentTokensSection } from "../components/EnrollmentTokensSection";
import { Message, Sheet } from "../components/SettingsKit";
import { SSHHubKeyCard } from "../components/SSHHubKeyCard";

/** Settings › Agents: where agents connect, enrollment tokens, agent keys and the hub's SSH key. */
export default function AgentSettingsPage() {
  const t = useTranslations("settings");
  const { user } = useAuth();

  if (!hasAdminRole(user?.role)) {
    return (
      <>
        <PageHeader title={t("agents.pageTitle")} />
        <Sheet pad>
          <p className="st-msg">{t("general.adminOnly")}</p>
        </Sheet>
      </>
    );
  }

  return <AgentSettings />;
}

function AgentSettings() {
  const t = useTranslations("settings");
  const enrollment = useEnrollment();

  return (
    <>
      <PageHeader title={t("agents.pageTitle")} />
      <div className="st">
        {enrollment.error ? (
          <div className="st-sec">
            <Message tone="bad">{enrollment.error}</Message>
          </div>
        ) : null}
        <AgentConnectionSection
          hubCandidates={enrollment.hubCandidates}
          hubURL={enrollment.hubURL}
          wsURL={enrollment.wsURL}
          selectHubURL={enrollment.selectHubURL}
        />
        <EnrollmentTokensSection
          enrollmentTokens={enrollment.enrollmentTokens}
          wsURL={enrollment.wsURL}
          newRawToken={enrollment.newRawToken}
          generating={enrollment.generating}
          generateToken={enrollment.generateToken}
          revokeEnrollmentToken={enrollment.revokeEnrollmentToken}
          clearNewToken={enrollment.clearNewToken}
        />
        <AgentKeysSection
          enrollmentTokens={enrollment.enrollmentTokens}
          agentTokens={enrollment.agentTokens}
          revokeAgentToken={enrollment.revokeAgentToken}
          cleanupDeadTokens={enrollment.cleanupDeadTokens}
        />
        <SSHHubKeyCard />
      </div>
    </>
  );
}
