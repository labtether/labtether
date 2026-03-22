"use client";

import { Button } from "../../../../components/ui/Button";
import type { AgentSettingsPayload } from "./agentSettingsModel";

type AgentSettingsOverviewSectionProps = {
  payload: AgentSettingsPayload;
  formatTime: (value?: string) => string;
  versionStatusClass: string;
  versionStatusLabel: string;
  hasPendingChanges: boolean;
  saving: boolean;
  onSave: () => void;
  onResetOverrides: () => void;
  testingDocker: boolean;
  onTestDocker: () => void;
  dockerTestOutput: string | null;
  forceAgentUpdate: boolean;
  onForceAgentUpdateChange: (checked: boolean) => void;
  updatingAgent: boolean;
  onUpdateAgent: () => void;
  agentUpdateOutput: string | null;
};

export function AgentSettingsOverviewSection({
  payload,
  formatTime,
  versionStatusClass,
  versionStatusLabel,
  hasPendingChanges,
  saving,
  onSave,
  onResetOverrides,
  testingDocker,
  onTestDocker,
  dockerTestOutput,
  forceAgentUpdate,
  onForceAgentUpdateChange,
  updatingAgent,
  onUpdateAgent,
  agentUpdateOutput,
}: AgentSettingsOverviewSectionProps) {
  return (
    <>
      <div className="mb-4 grid gap-2 text-xs text-[var(--muted)] md:grid-cols-3">
        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
          <p className="font-medium text-[var(--text)]">Device Fingerprint</p>
          <p className="mt-1 break-all">{payload.state?.fingerprint || payload.fingerprint || "Not reported"}</p>
        </div>
        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
          <p className="font-medium text-[var(--text)]">Apply State</p>
          <p className="mt-1">
            Status: <span className="text-[var(--text)]">{payload.state?.status || "unknown"}</span>
          </p>
          <p>Last update: {formatTime(payload.state?.updated_at)}</p>
          <p>Last apply: {formatTime(payload.state?.applied_at)}</p>
          {payload.state?.last_error ? <p className="text-[var(--bad)]">Error: {payload.state.last_error}</p> : null}
        </div>
        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
          <p className="font-medium text-[var(--text)]">Agent Version</p>
          <p className="mt-1">
            Status: <span className={versionStatusClass}>{versionStatusLabel}</span>
          </p>
          <p>Current: <span className="text-[var(--text)]">{payload.agent_version || "Unknown"}</span></p>
          <p>Latest: <span className="text-[var(--text)]">{payload.latest_agent_version || "Unknown"}</span></p>
          {payload.latest_agent_published_at ? <p>Published: {formatTime(payload.latest_agent_published_at)}</p> : null}
          {(payload.agent_platform || payload.agent_arch) ? (
            <p>Target: {payload.agent_platform || "unknown"} / {payload.agent_arch || "unknown"}</p>
          ) : null}
          {payload.agent_version_error ? <p className="text-[var(--warn)]">{payload.agent_version_error}</p> : null}
        </div>
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" onClick={onSave} disabled={!hasPendingChanges || saving}>
          {saving ? "Saving..." : "Save Changes"}
        </Button>
        <Button variant="secondary" onClick={onResetOverrides} disabled={saving}>
          Reset Overrides
        </Button>
        <Button
          variant="ghost"
          onClick={onTestDocker}
          disabled={testingDocker || !payload.connected}
        >
          {testingDocker ? "Testing Docker..." : "Test Docker Endpoint"}
        </Button>
        {!payload.connected ? (
          <span className="text-xs text-[var(--muted)]">Connect the agent to run remote docker test.</span>
        ) : null}
      </div>

      {dockerTestOutput ? (
        <pre className="mb-4 max-h-40 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3 text-xs text-[var(--text)] whitespace-pre-wrap">
          {dockerTestOutput}
        </pre>
      ) : null}

      <div className="mb-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
        <p className="text-sm font-medium text-[var(--text)]">Agent Binary Update</p>
        <p className="mt-1 text-xs text-[var(--muted)]">
          Trigger a self-update check for this device. The agent may disconnect briefly if an update is applied.
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <label className="inline-flex items-center gap-2 text-xs text-[var(--muted)]">
            <input
              type="checkbox"
              checked={forceAgentUpdate}
              onChange={(event) => onForceAgentUpdateChange(event.target.checked)}
              disabled={updatingAgent || !payload.connected}
            />
            Force reinstall
          </label>
          <Button
            variant={forceAgentUpdate ? "danger" : "secondary"}
            onClick={onUpdateAgent}
            disabled={updatingAgent || !payload.connected}
          >
            {updatingAgent ? "Updating Agent..." : forceAgentUpdate ? "Force Reinstall Agent" : "Check & Update Agent"}
          </Button>
          {!payload.connected ? (
            <span className="text-xs text-[var(--muted)]">Connect the agent to run self-update.</span>
          ) : null}
        </div>
      </div>

      {agentUpdateOutput ? (
        <pre className="mb-4 max-h-40 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3 text-xs text-[var(--text)] whitespace-pre-wrap">
          {agentUpdateOutput}
        </pre>
      ) : null}
    </>
  );
}
