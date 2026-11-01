"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import type { AgentSettingsPayload } from "./agentSettingsModel";

type AgentSettingsOverviewSectionProps = {
  payload: AgentSettingsPayload;
  formatTime: (value?: string) => string;
  testingDocker: boolean;
  onTestDocker: () => void;
  dockerTestOutput: string | null;
  forceAgentUpdate: boolean;
  onForceAgentUpdateChange: (checked: boolean) => void;
  updatingAgent: boolean;
  onUpdateAgent: () => void;
  agentUpdateOutput: string | null;
};

const APPLY_WORDS = new Set(["applied", "pending", "failed", "error", "unknown"]);

/** The agent as key-value rows (version, platform, key, settings state) with Test Docker and Update in the foot. */
export function AgentSettingsOverviewSection({
  payload,
  formatTime,
  testingDocker,
  onTestDocker,
  dockerTestOutput,
  forceAgentUpdate,
  onForceAgentUpdateChange,
  updatingAgent,
  onUpdateAgent,
  agentUpdateOutput,
}: AgentSettingsOverviewSectionProps) {
  const t = useTranslations("devices.settingsTab.agent");
  const offline = !payload.connected;
  const fingerprint = payload.state?.fingerprint || payload.fingerprint || "";
  const platform = [payload.agent_platform, payload.agent_arch].filter(Boolean).join(" · ");
  const applyStatus = (payload.state?.status || "unknown").toLowerCase();
  const applyWord = APPLY_WORDS.has(applyStatus) ? t(`apply.${applyStatus === "error" ? "failed" : applyStatus}`) : payload.state?.status;
  const updateAvailable = payload.agent_version_status === "update_available";
  const updateLabel = updatingAgent
    ? t("updating")
    : forceAgentUpdate
      ? t("reinstall")
      : updateAvailable && payload.latest_agent_version
        ? t("updateTo", { version: payload.latest_agent_version })
        : t("checkUpdate");

  return (
    <div className="dv-sheet tight dst-agent">
      <dl className="dv-kv">
        <div>
          <dt>{t("version")}</dt>
          <dd>
            <span className="num">{payload.agent_version || t("unknown")}</span>
            {payload.agent_version_status === "up_to_date" ? <span className="dv-grey">{t("latest")}</span> : null}
            {updateAvailable && payload.latest_agent_version ? (
              <span
                className="dst-warn"
                title={payload.latest_agent_published_at ? t("published", { time: formatTime(payload.latest_agent_published_at) }) : undefined}
              >
                {t("available", { version: payload.latest_agent_version })}
              </span>
            ) : null}
            {payload.agent_version_error ? <span className="dst-warn dst-block">{payload.agent_version_error}</span> : null}
          </dd>
        </div>
        {platform ? (
          <div>
            <dt>{t("platform")}</dt>
            <dd><span>{platform}</span></dd>
          </div>
        ) : null}
        <div>
          <dt>{t("key")}</dt>
          <dd>{fingerprint ? <span className="dv-mono dst-key">{fingerprint}</span> : <span className="dv-grey">{t("notReported")}</span>}</dd>
        </div>
        <div>
          <dt>{t("settingsState")}</dt>
          <dd>
            <span className={applyStatus === "failed" || applyStatus === "error" ? "dst-bad" : undefined}>{applyWord}</span>
            {payload.state?.applied_at ? <span className="dv-grey">{t("appliedAt", { time: formatTime(payload.state.applied_at) })}</span> : null}
            {payload.state?.updated_at ? <span className="dv-grey">{t("changedAt", { time: formatTime(payload.state.updated_at) })}</span> : null}
            {payload.state?.last_error ? <span className="dst-bad dst-block">{payload.state.last_error}</span> : null}
          </dd>
        </div>
      </dl>

      {dockerTestOutput ? <pre className="dst-out" aria-label={t("dockerOutput")}>{dockerTestOutput}</pre> : null}
      {agentUpdateOutput ? <pre className="dst-out" aria-label={t("updateOutput")}>{agentUpdateOutput}</pre> : null}

      <div className="dv-sheet-foot">
        {offline ? <span className="dv-hint">{t("offline")}</span> : null}
        <span className="dv-sp dst-grow" />
        <label className="dst-check">
          <input
            type="checkbox"
            checked={forceAgentUpdate}
            onChange={(event) => onForceAgentUpdateChange(event.target.checked)}
            disabled={updatingAgent || offline}
          />
          {t("forceReinstall")}
        </label>
        <Button size="sm" onClick={onTestDocker} disabled={testingDocker || offline}>
          {testingDocker ? t("testingDocker") : t("testDocker")}
        </Button>
        <Button size="sm" variant={forceAgentUpdate ? "danger" : "secondary"} onClick={onUpdateAgent} disabled={updatingAgent || offline}>
          {updateLabel}
        </Button>
      </div>
    </div>
  );
}
