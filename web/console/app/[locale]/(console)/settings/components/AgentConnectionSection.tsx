"use client";

import { useTranslations } from "next-intl";
import { Select } from "../../../../components/ui/Input";
import type { HubConnectionCandidate } from "../../../../hooks/useEnrollment";
import { CopyButton, Row, Section, Sheet } from "./SettingsKit";

type AgentConnectionSectionProps = {
  hubCandidates: HubConnectionCandidate[];
  hubURL: string;
  wsURL: string;
  selectHubURL: (hubURL: string) => void;
};

function candidateLabel(candidate: HubConnectionCandidate): string {
  const label = candidate.label || (candidate.kind === "tailscale" ? "Tailscale" : candidate.kind === "lan" ? "LAN" : candidate.kind);
  return candidate.host ? `${label} (${candidate.host})` : label;
}

/** Where agents connect: the hub address and its WebSocket, and which of the hub's addresses new tokens use. */
export function AgentConnectionSection({ hubCandidates, hubURL, wsURL, selectHubURL }: AgentConnectionSectionProps) {
  const t = useTranslations("settings");
  const selected = hubCandidates.find((candidate) => candidate.hub_url === hubURL) ?? null;

  return (
    <Section title={t("agents.connection")} id="connection">
      <Sheet>
        {hubCandidates.length > 1 ? (
          <Row
            label={<label htmlFor="st-hub-target">{t("agents.connectionTarget")}</label>}
            hint={selected?.preferred_reason}
            two
          >
            <Select id="st-hub-target" className="st-in-l" value={hubURL} onChange={(event) => selectHubURL(event.target.value)}>
              {hubCandidates.map((candidate) => (
                <option key={candidate.hub_url} value={candidate.hub_url}>{candidateLabel(candidate)}</option>
              ))}
            </Select>
          </Row>
        ) : null}
        <Row label={t("agents.hubUrl")}>
          <code className="st-mono">{hubURL || "—"}</code>
          {hubURL ? <CopyButton text={hubURL} label={t("agents.copyHubUrlAriaLabel")} copiedLabel={t("agents.copied")} /> : null}
        </Row>
        <Row label={t("agents.webSocketUrl")}>
          <code className="st-mono">{wsURL || "—"}</code>
          {wsURL ? <CopyButton text={wsURL} label={t("agents.copyWebSocketUrlAriaLabel")} copiedLabel={t("agents.copied")} /> : null}
        </Row>
      </Sheet>
    </Section>
  );
}
