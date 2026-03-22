"use client";

import type { Dispatch, SetStateAction } from "react";
import { useMemo, useState } from "react";
import { Copy, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import type { EnrollmentToken, AgentTokenSummary } from "../../../../console/models";
import type { HubConnectionCandidate } from "../../../../hooks/useEnrollment";
import { formatAge } from "../../../../console/formatters";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { SkeletonRow } from "../../../../components/ui/Skeleton";

type ConnectAgentsCardProps = {
  sectionHeadingClassName: string;
  enrollmentTokens: EnrollmentToken[];
  agentTokens: AgentTokenSummary[];
  hubCandidates: HubConnectionCandidate[];
  hubURL: string;
  wsURL: string;
  selectHubURL: (hubURL: string) => void;
  enrollLoading: boolean;
  enrollError: string;
  newRawToken: string;
  generating: boolean;
  generateToken: (label: string, ttlHours: number, maxUses: number) => Promise<void> | void;
  revokeEnrollmentToken: (id: string) => Promise<void> | void;
  revokeAgentToken: (id: string) => Promise<void> | void;
  cleanupDeadTokens: () => Promise<{ enrollment_deleted: number; agent_deleted: number } | null>;
  clearNewToken: () => void;
  tokenLabel: string;
  setTokenLabel: Dispatch<SetStateAction<string>>;
  tokenTTL: number;
  setTokenTTL: Dispatch<SetStateAction<number>>;
  tokenMaxUses: number;
  setTokenMaxUses: Dispatch<SetStateAction<number>>;
  copyToClipboard: (text: string, label: string, toastMessage?: string) => void;
  copied: string;
};

export function ConnectAgentsCard({
  sectionHeadingClassName,
  enrollmentTokens,
  agentTokens,
  hubCandidates,
  hubURL,
  wsURL,
  selectHubURL,
  enrollLoading,
  enrollError,
  newRawToken,
  generating,
  generateToken,
  revokeEnrollmentToken,
  revokeAgentToken,
  cleanupDeadTokens,
  clearNewToken,
  tokenLabel,
  setTokenLabel,
  tokenTTL,
  setTokenTTL,
  tokenMaxUses,
  setTokenMaxUses,
  copyToClipboard,
  copied,
}: ConnectAgentsCardProps) {
  const t = useTranslations("settings");
  const [cleaning, setCleaning] = useState(false);
  const [cleanupResult, setCleanupResult] = useState("");

  const { deadEnrollmentCount, deadAgentCount, totalDead } = useMemo(() => {
    const now = new Date();
    const deadEnroll = enrollmentTokens.filter((tok) => {
      return !!tok.revoked_at || new Date(tok.expires_at) < now || (tok.max_uses > 0 && tok.use_count >= tok.max_uses);
    }).length;
    const deadAgent = agentTokens.filter((tok) => {
      return tok.status === "revoked" && !tok.last_used_at;
    }).length;
    return { deadEnrollmentCount: deadEnroll, deadAgentCount: deadAgent, totalDead: deadEnroll + deadAgent };
  }, [enrollmentTokens, agentTokens]);
  const selectedCandidate = useMemo(
    () => hubCandidates.find((candidate) => candidate.hub_url === hubURL) ?? null,
    [hubCandidates, hubURL],
  );

  const handleCleanup = async () => {
    if (!window.confirm(t("agents.deleteCount", { count: totalDead }))) return;
    setCleaning(true);
    setCleanupResult("");
    const result = await cleanupDeadTokens();
    setCleaning(false);
    if (result) {
      const total = result.enrollment_deleted + result.agent_deleted;
      setCleanupResult(t("agents.clearDeadTokens", { count: total }));
      setTimeout(() => setCleanupResult(""), 3000);
    }
  };

  return (
    <Card className="mb-6">
      <h2>{t("agents.title")}</h2>
      {enrollLoading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}
      {enrollError ? <p className="text-xs text-[var(--bad)]">{enrollError}</p> : null}

      <div className="space-y-6">
        <div>
          <p className={sectionHeadingClassName}>{t("agents.connection")}</p>
          {hubCandidates.length > 1 ? (
            <div className="mt-2 max-w-xl">
              <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                {t("agents.connectionTarget")}
                <Select value={hubURL} onChange={(event) => selectHubURL(event.target.value)}>
                  {hubCandidates.map((candidate) => {
                    const label = candidate.label || (candidate.kind === "tailscale" ? "Tailscale" : candidate.kind === "lan" ? "LAN" : "Connection");
                    const optionText = candidate.host ? `${label} (${candidate.host})` : label;
                    return (
                      <option key={candidate.hub_url} value={candidate.hub_url}>
                        {optionText}
                      </option>
                    );
                  })}
                </Select>
              </label>
            </div>
          ) : null}
          {selectedCandidate?.preferred_reason ? (
            <p className="mt-2 max-w-3xl text-xs text-[var(--muted)]">{selectedCandidate.preferred_reason}</p>
          ) : null}
          <div className="divide-y divide-[var(--line)] mt-2">
            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">{t("agents.hubUrl")}</span>
                <span className="text-xs text-[var(--muted)]">{t("agents.hubUrlDescription")}</span>
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">
                <code className="block max-w-full break-all text-xs sm:text-sm">{hubURL || "—"}</code>
                {hubURL ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => copyToClipboard(hubURL, "hub", t("agents.hubUrl") + " " + t("agents.copied").toLowerCase())}
                    aria-label={t("agents.copyHubUrlAriaLabel")}
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "hub" ? t("agents.copied") : t("agents.copy")}
                  </Button>
                ) : null}
              </div>
            </div>
            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">{t("agents.webSocketUrl")}</span>
                <span className="text-xs text-[var(--muted)]">{t("agents.webSocketUrlDescription")}</span>
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">
                <code className="block max-w-full break-all text-xs sm:text-sm">{wsURL || "—"}</code>
                {wsURL ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => copyToClipboard(wsURL, "ws", t("agents.webSocketUrl") + " " + t("agents.copied").toLowerCase())}
                    aria-label={t("agents.copyWebSocketUrlAriaLabel")}
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "ws" ? t("agents.copied") : t("agents.copy")}
                  </Button>
                ) : null}
              </div>
            </div>
          </div>
        </div>

        {newRawToken ? (
          <div>
            <p className={sectionHeadingClassName}>{t("agents.newToken")}</p>
            <Card className="bg-[var(--ok-glow)] border-[var(--ok)]/20 space-y-3 mt-2">
              <p className="text-sm font-medium text-[var(--text)]">{t("agents.newTokenDescription")}</p>
              <div className="flex flex-wrap items-center gap-2">
                <code className="block max-w-full break-all text-xs sm:text-sm">{newRawToken}</code>
                <Button
                  variant="primary"
                  onClick={() => copyToClipboard(newRawToken, "token", t("copiedToClipboard"))}
                >
                  <Copy size={13} className="shrink-0" />
                  {copied === "token" ? t("agents.copied") : t("agents.copyToken")}
                </Button>
                <Button variant="ghost" size="sm" onClick={clearNewToken}>{t("agents.dismiss")}</Button>
              </div>
              <details className="text-xs text-[var(--muted)]">
                <summary className="cursor-pointer hover:text-[var(--text)]">{t("agents.copyPasteSetup")}</summary>
                <pre className="mt-2 p-3 bg-[var(--surface)] rounded-lg overflow-x-auto text-xs">{`# Docker
docker run -e LABTETHER_ENROLLMENT_TOKEN="${newRawToken}" \\
  -e LABTETHER_WS_URL="${wsURL}" \\
  labtether/labtether-agent

# Bare metal (Linux/FreeBSD)
export LABTETHER_ENROLLMENT_TOKEN="${newRawToken}"
export LABTETHER_WS_URL="${wsURL}"
./labtether-agent

# macOS / Windows: configure in the app settings`}</pre>
              </details>
            </Card>
          </div>
        ) : null}

        <div>
          <p className={sectionHeadingClassName}>{t("agents.createEnrollmentToken")}</p>
          <div className="space-y-2 mt-2">
            <p className="text-xs text-[var(--muted)]">{t("agents.createEnrollmentTokenDescription")}</p>
            <div className="flex flex-wrap items-end gap-3">
              <Input
                placeholder={t("agents.labelPlaceholder")}
                value={tokenLabel}
                onChange={(e) => setTokenLabel(e.target.value)}
              />
              <label className="flex flex-col gap-0.5 text-xs text-[var(--muted)]">
                <span className="text-xs text-[var(--muted)]">{t("agents.validForHours")}</span>
                <Input
                  className="w-20"
                  type="number"
                  min={1}
                  max={720}
                  value={tokenTTL}
                  onChange={(e) => setTokenTTL(Number(e.target.value))}
                />
              </label>
              <label className="flex flex-col gap-0.5 text-xs text-[var(--muted)]">
                <span className="text-xs text-[var(--muted)]">{t("agents.maxUses")}</span>
                <Input
                  className="w-20"
                  type="number"
                  min={1}
                  value={tokenMaxUses}
                  onChange={(e) => setTokenMaxUses(Number(e.target.value))}
                />
              </label>
              <Button
                variant="primary"
                disabled={generating}
                onClick={() => void generateToken(tokenLabel, tokenTTL, tokenMaxUses)}
              >
                {generating ? t("agents.generating") : t("agents.createToken")}
              </Button>
            </div>
          </div>
        </div>

        {enrollmentTokens.length > 0 ? (
          <div>
            <p className={sectionHeadingClassName}>{t("agents.savedEnrollmentTokens")}</p>
            <ul className="divide-y divide-[var(--line)] mt-2">
              {enrollmentTokens.map((tok) => {
                const isExpired = new Date(tok.expires_at) < new Date();
                const isDisabled = !!tok.revoked_at;
                const isExhausted = tok.max_uses > 0 && tok.use_count >= tok.max_uses;
                const statusLabel = isDisabled ? "revoked" : isExpired ? "expired" : isExhausted ? "exhausted" : "active";

                return (
                  <li key={tok.id} className="flex flex-wrap items-center gap-3 py-2">
                    <span className="text-sm font-medium text-[var(--text)]">{tok.label || tok.id}</span>
                    <span className="text-xs text-[var(--muted)]">{tok.use_count}{tok.max_uses > 0 ? `/${tok.max_uses}` : ""} uses</span>
                    <span className="text-xs text-[var(--muted)]">{t("agents.expires", { age: formatAge(tok.expires_at) })}</span>
                    <Badge status={statusLabel} size="sm" />
                    {!isDisabled ? (
                      <Button variant="ghost" size="sm" onClick={() => void revokeEnrollmentToken(tok.id)}>{t("agents.disable")}</Button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}

        {agentTokens.length > 0 ? (
          <div>
            <p className={sectionHeadingClassName}>{t("agents.connectedAgents")}</p>
            <ul className="divide-y divide-[var(--line)] mt-2">
              {agentTokens.map((tok) => {
                return (
                  <li key={tok.id} className="flex flex-wrap items-center gap-3 py-2">
                    <span className="text-sm font-medium text-[var(--text)]">{tok.asset_id}</span>
                    <Badge status={tok.status} size="sm" />
                    {tok.last_used_at ? <span className="text-xs text-[var(--muted)]">{t("agents.lastSeen", { age: formatAge(tok.last_used_at) })}</span> : null}
                    <span className="text-xs text-[var(--muted)]">{t("agents.added", { age: formatAge(tok.created_at) })}</span>
                    {tok.device_fingerprint ? (
                      <span className="text-xs text-[var(--muted)]">
                        {t("agents.fingerprint")}
                        {" "}
                        <code className="break-all">{tok.device_fingerprint}</code>
                      </span>
                    ) : null}
                    {tok.status === "active" ? (
                      <Button variant="ghost" size="sm" onClick={() => void revokeAgentToken(tok.id)}>{t("agents.disable")}</Button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}

        {totalDead > 0 ? (
          <div className="flex items-center gap-3 pt-2 border-t border-[var(--line)]">
            <Button
              variant="ghost"
              size="sm"
              disabled={cleaning}
              onClick={() => void handleCleanup()}
              className="text-[var(--bad)]"
            >
              <Trash2 size={13} className="shrink-0" />
              {cleaning ? t("agents.clearing") : t("agents.clearDeadTokens", { count: totalDead })}
            </Button>
            {cleanupResult ? <span className="text-xs text-[var(--ok)]">{cleanupResult}</span> : null}
          </div>
        ) : null}
      </div>
    </Card>
  );
}
