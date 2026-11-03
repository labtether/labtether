"use client";

import { type Dispatch, type SetStateAction } from "react";
import { useTranslations } from "next-intl";
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
  const t = useTranslations("auth.addDevice");
  return (
    <div className="space-y-3 rounded-lg border border-[var(--line)] p-3">
      <div>
        <p className="text-xs font-medium text-[var(--text)] mb-1">
          {t("linux.title")}
        </p>
        {selectedCandidate?.bootstrap_strategy ===
        "pinned_ca_bootstrap" ? (
          <p className="mt-1 text-xs text-[var(--warn)]">{t("linux.pinnedCa")}</p>
        ) : selectedCandidate?.trust_mode === "custom_tls" ? (
          <p className="mt-1 text-xs text-[var(--muted)]">{t("linux.customTls")}</p>
        ) : null}
      </div>

      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
          {t("linux.fileAccess")}
          <Select
            value={linuxOptions.filesRootMode}
            onChange={(event) =>
              setLinuxOptions((current) => ({
                ...current,
                filesRootMode: event.target.value as FilesRootMode,
              }))
            }
          >
            <option value="home">{t("linux.fileHome")}</option>
            <option value="full">{t("linux.fileFull")}</option>
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
          {t("linux.vnc")}
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
          {t("linux.autoUpdate")}
        </label>
      </div>

      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]">
        <button
          type="button"
          onClick={() => setShowAdvancedSettings((current) => !current)}
          aria-expanded={showAdvancedSettings}
          className="flex w-full items-center justify-between px-3 py-2 text-left"
        >
          <span className="block text-xs font-medium text-[var(--text)]">
            {t("linux.advanced")}
          </span>
          <span className="text-xs text-[var(--muted)]">
            {showAdvancedSettings ? t("linux.hide") : t("linux.show")}
          </span>
        </button>

        {showAdvancedSettings ? (
          <div className="grid grid-cols-1 gap-3 border-t border-[var(--line)] px-3 py-3 md:grid-cols-2">
            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              {t("linux.docker")}
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
                <option value="auto">{t("linux.dockerAuto")}</option>
                <option value="true">{t("linux.dockerOn")}</option>
                <option value="false">{t("linux.dockerOff")}</option>
              </Select>
            </label>

            <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
              {t("linux.dockerInterval")}
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
              {t("linux.dockerEndpoint")}
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
              {t("linux.forceUpdate")}
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
              {t("linux.askToken")}
            </label>

            <div className="md:col-span-2">
              <p className="text-xs font-medium text-[var(--muted)] mb-1">
                {t("linux.binaryCommand")}
              </p>
              <div className="relative bg-[var(--panel)] rounded-lg px-3 py-2">
                <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">
                  {fallbackCommand}
                </pre>
                <div className="absolute top-2 right-2">
                  <CopyButton
                    text={fallbackCommand}
                    label={t("linux.copyBinaryCommand")}
                  />
                </div>
              </div>
            </div>
          </div>
        ) : null}
      </div>

      {!linuxOptions.includeEnrollmentToken ? (
        <p className="text-xs text-[var(--warn)]">{t("linux.noToken")}</p>
      ) : null}

      <div>
        <p className="text-xs font-medium text-[var(--muted)] mb-1">
          {t("linux.runOnLinux")}
        </p>
        <div className="relative bg-[var(--surface)] rounded-lg px-3 py-2">
          <pre className="text-xs text-[var(--text)] whitespace-pre-wrap">
            {installerCommand}
          </pre>
          <div className="absolute top-2 right-2">
            <CopyButton
              text={installerCommand}
              label={t("linux.copyInstall")}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
