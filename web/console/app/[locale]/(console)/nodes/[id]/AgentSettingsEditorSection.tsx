"use client";

import { Badge } from "../../../../components/ui/Badge";
import { AgentSettingInputControl } from "./AgentSettingInputControl";
import type { AgentSettingEntry, AgentSettingsHistoryEvent } from "./agentSettingsModel";

type AgentSettingsEditorSectionProps = {
  serviceDiscoverySettings: AgentSettingEntry[];
  generalSettings: AgentSettingEntry[];
  draft: Record<string, string>;
  settingKeyFilesRootMode: string;
  settingControlSourceStatus: (source: string) => string;
  onUpdateDraftValue: (key: string, value: string) => void;
  history: AgentSettingsHistoryEvent[];
  formatTime: (value?: string) => string;
};

export function AgentSettingsEditorSection({
  serviceDiscoverySettings,
  generalSettings,
  draft,
  settingKeyFilesRootMode,
  settingControlSourceStatus,
  onUpdateDraftValue,
  history,
  formatTime,
}: AgentSettingsEditorSectionProps) {
  return (
    <>
      {serviceDiscoverySettings.length > 0 ? (
        <div className="mb-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
          <p className="text-sm font-medium text-[var(--text)]">Service Discovery Policy</p>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Per-node discovery overrides. Apply to 1-2 canary nodes first, verify Services output, then scale rollout.
          </p>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Keep LAN scan disabled unless required. When enabled, use narrow private CIDRs and minimal port lists.
          </p>
          <div className="mt-3 divide-y divide-[var(--line)]">
            {serviceDiscoverySettings.map((setting) => {
              const currentValue = draft[setting.key] ?? setting.effective_value;
              const editable = setting.hub_managed && !setting.local_only;
              return (
                <div key={setting.key} className="grid gap-2 py-2 md:grid-cols-[minmax(0,1fr)_260px] md:items-start">
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-[var(--text)]">{setting.label}</p>
                    <p className="text-xs text-[var(--muted)]">{setting.description}</p>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--muted)]">
                      <span>Key: {setting.key}</span>
                      <Badge status={settingControlSourceStatus(setting.source)} size="sm" />
                      {setting.restart_required ? <span>Restart required</span> : null}
                    </div>
                  </div>
                  <div className="w-full">
                    <AgentSettingInputControl
                      setting={setting}
                      currentValue={currentValue}
                      editable={editable}
                      onChange={onUpdateDraftValue}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : null}

      <div className="divide-y divide-[var(--line)]">
        {generalSettings.map((setting) => {
          const currentValue = draft[setting.key] ?? setting.effective_value;
          const editable = setting.hub_managed && !setting.local_only;
          return (
            <div key={setting.key} className="grid gap-2 py-3 md:grid-cols-[minmax(0,1fr)_280px] md:items-start">
              <div className="min-w-0">
                <p className="text-sm font-medium text-[var(--text)]">{setting.label}</p>
                <p className="text-xs text-[var(--muted)]">{setting.description}</p>
                {setting.key === settingKeyFilesRootMode && currentValue === "full" ? (
                  <p className="mt-1 text-xs text-[var(--warn)]">
                    Full mode allows browsing the entire filesystem on this agent. Use only on trusted hosts.
                  </p>
                ) : null}
                <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--muted)]">
                  <span>Key: {setting.key}</span>
                  <Badge status={settingControlSourceStatus(setting.source)} size="sm" />
                  {setting.restart_required ? <span>Restart required</span> : null}
                  {setting.drift ? <span className="text-[var(--warn)]">Runtime drift detected</span> : null}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-[var(--muted)]">
                  <span>Default: {setting.default_value}</span>
                  {setting.global_value ? <span>Global: {setting.global_value}</span> : null}
                  {setting.override_value ? <span>Override: {setting.override_value}</span> : null}
                  {setting.state_value ? <span>Reported: {setting.state_value}</span> : null}
                </div>
              </div>
              <div className="w-full">
                <AgentSettingInputControl
                  setting={setting}
                  currentValue={currentValue}
                  editable={editable}
                  onChange={onUpdateDraftValue}
                />
              </div>
            </div>
          );
        })}
      </div>

      <div className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
        <p className="mb-2 text-xs font-medium text-[var(--text)]">Recent Apply History</p>
        {history.length === 0 ? (
          <p className="text-xs text-[var(--muted)]">No apply history reported yet.</p>
        ) : (
          <ul className="space-y-2 text-xs text-[var(--muted)]">
            {history.slice(0, 5).map((event, index) => (
              <li key={`${event.revision || "rev"}-${index}`} className="rounded-md border border-[var(--line)] p-2">
                <p>
                  Status: <span className="text-[var(--text)]">{event.status || "unknown"}</span>
                </p>
                <p>Updated: {formatTime(event.updated_at)}</p>
                <p>Applied: {formatTime(event.applied_at)}</p>
                {event.revision ? <p>Revision: {event.revision}</p> : null}
                {event.last_error ? <p className="text-[var(--bad)]">Error: {event.last_error}</p> : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}
