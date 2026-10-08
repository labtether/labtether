"use client";

import { type Dispatch, type SetStateAction } from "react";
import { Input, Select } from "../ui/Input";
import { type HubConnectionCandidate } from "../../hooks/useEnrollment";
import { type LinuxInstallOptions, type FilesRootMode, type LinuxDockerMode } from "./agentInstallCommands";
import { CopyButton } from "./EnrollmentCopyButton";

export type LinuxAgentInstallFormProps = {
  selectedCandidate: HubConnectionCandidate | null;
  linuxOptions: LinuxInstallOptions;
  setLinuxOptions: Dispatch<SetStateAction<LinuxInstallOptions>>;
  showAdvancedSettings: boolean;
  setShowAdvancedSettings: Dispatch<SetStateAction<boolean>>;
  fallbackCommand: string;
  installerCommand: string;
};

export function LinuxAgentInstallForm({
  selectedCandidate,
  linuxOptions,
  setLinuxOptions,
  showAdvancedSettings,
  setShowAdvancedSettings,
  fallbackCommand,
  installerCommand,
}: LinuxAgentInstallFormProps) {
  return (
    <div className="space-y-3 rounded-lg border border-[var(--line)] p-3">
      <div>
        <p className="text-xs font-medium text-[var(--text)] mb-1">
          Linux Installer Script (Recommended)
        </p>
        <p className="text-xs text-[var(--muted)]">
          Choose the normal access settings here, then run the generated
          installer.
        </p>
        {selectedCandidate?.bootstrap_strategy ===
        "pinned_ca_bootstrap" ? (
          <p className="mt-1 text-xs text-[var(--warn)]">
            This target uses LabTether&apos;s built-in CA, so the
            generated command bootstraps trust before running the
            installer.
          </p>
        ) : selectedCandidate?.trust_mode === "custom_tls" ? (
          <p className="mt-1 text-xs text-[var(--muted)]">
            This target uses operator-managed TLS. Make sure the
            uploaded certificate chain is already trusted by the target
            machine.
          </p>
        ) : null}
      </div>

      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
          File access
          <Select
            value={linuxOptions.filesRootMode}
            onChange={(event) =>
              setLinuxOptions((current) => ({
                ...current,
                filesRootMode: event.target.value as FilesRootMode,
              }))
            }
          >
            <option value="home">Home-only access</option>
            <option value="full">Full disk access</option>
          </Select>
        </label>

        <label className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)]">
          <input
            type="checkbox"
            checked={linuxOptions.autoInstallVNC}
            onChange={(event) =>
              setLinuxOptions((current) => ({
                ...current,
                autoInstallVNC: event.target.checked,
              }))
            }
          />
          Enable desktop prerequisites for VNC/remote view
        </label>

        <label className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)] md:col-span-2">
          <input
            type="checkbox"
            checked={linuxOptions.autoUpdateEnabled}
            onChange={(event) =>
              setLinuxOptions((current) => ({
                ...current,
                autoUpdateEnabled: event.target.checked,
              }))
            }
          />
          Keep the agent updated automatically
        </label>
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]">
        <button
          type="button"
          onClick={() => setShowAdvancedSettings((current) => !current)}
          className="flex w-full items-center justify-between px-3 py-2 text-left"
        >
          <span>
            <span className="block text-xs font-medium text-[var(--text)]">
              Advanced settings
            </span>
            <span className="block text-[11px] text-[var(--muted)]">
              Docker discovery, force update, manual token control, and
              fallback binary install.
            </span>
          </span>
          <span className="text-xs text-[var(--muted)]">
            {showAdvancedSettings ? "Hide" : "Show"}
          </span>
        </button>

        {showAdvancedSettings ? (
          <div className="grid grid-cols-1 gap-3 border-t border-[var(--line)] px-3 py-3 md:grid-cols-2">
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Docker mode
              <Select
                value={linuxOptions.dockerEnabled}
                onChange={(event) =>
                  setLinuxOptions((current) => ({
                    ...current,
                    dockerEnabled: event.target
                      .value as LinuxDockerMode,
                  }))
                }
              >
                <option value="auto">auto</option>
                <option value="true">true</option>
                <option value="false">false</option>
              </Select>
            </label>

            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              Docker discovery interval (sec)
              <Input
                type="number"
                min={5}
                max={3600}
                value={linuxOptions.dockerDiscoveryIntervalSec}
                disabled={linuxOptions.dockerEnabled === "false"}
                onChange={(event) =>
                  setLinuxOptions((current) => ({
                    ...current,
                    dockerDiscoveryIntervalSec: event.target.value,
                  }))
                }
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-[var(--muted)] md:col-span-2">
              Docker endpoint
              <Input
                value={linuxOptions.dockerEndpoint}
                disabled={linuxOptions.dockerEnabled === "false"}
                onChange={(event) =>
                  setLinuxOptions((current) => ({
                    ...current,
                    dockerEndpoint: event.target.value,
                  }))
                }
                placeholder="/var/run/docker.sock"
              />
            </label>

            <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
              <input
                type="checkbox"
                checked={linuxOptions.forceUpdate}
                onChange={(event) =>
                  setLinuxOptions((current) => ({
                    ...current,
                    forceUpdate: event.target.checked,
                  }))
                }
              />
              Force update immediately after install
            </label>

            <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
              <input
                type="checkbox"
                checked={linuxOptions.includeEnrollmentToken}
                onChange={(event) =>
                  setLinuxOptions((current) => ({
                    ...current,
                    includeEnrollmentToken: event.target.checked,
                  }))
                }
              />
              Prompt for one-time enrollment token
            </label>

            <div className="md:col-span-2">
              <p className="text-xs font-medium text-[var(--muted)] mb-1">
                Manual binary command
              </p>
              <div className="relative bg-[var(--panel)] rounded-lg px-3 py-2">
                <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">
                  {fallbackCommand}
                </pre>
                <div className="absolute top-2 right-2">
                  <CopyButton
                    text={fallbackCommand}
                    label="manual binary command"
                  />
                </div>
              </div>
            </div>
          </div>
        ) : null}
      </div>

      {!linuxOptions.includeEnrollmentToken ? (
        <p className="text-xs text-[var(--warn)]">
          Token disabled: this install uses pending approval flow
          instead of auto-enrollment.
        </p>
      ) : null}

      <div>
        <p className="text-xs font-medium text-[var(--muted)] mb-1">
          Run on Linux target
        </p>
        <div className="relative bg-[var(--surface)] rounded-lg px-3 py-2">
          <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">
            {installerCommand}
          </pre>
          <div className="absolute top-2 right-2">
            <CopyButton
              text={installerCommand}
              label="Linux installer command"
            />
          </div>
        </div>
      </div>
    </div>
  );
}
