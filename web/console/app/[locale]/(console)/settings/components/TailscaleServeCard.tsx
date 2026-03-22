"use client";

import type { Dispatch, ReactNode, SetStateAction } from "react";
import { useState } from "react";
import { Copy, RefreshCw, Sparkles } from "lucide-react";

import { runtimeSettingKeys, type RuntimeSettingEntry } from "../../../../console/models";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input, Select } from "../../../../components/ui/Input";
import { SkeletonRow } from "../../../../components/ui/Skeleton";
import { useTailscaleServeStatus } from "../../../../hooks/useTailscaleServeStatus";

type TailscaleServeCardProps = {
  copyToClipboard: (text: string, label: string, toastMessage?: string) => void;
  copied: string;
  runtimeSettings: RuntimeSettingEntry[];
  runtimeDraftValues: Record<string, string>;
  setRuntimeDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  runtimeSettingsLoading: boolean;
  runtimeSettingsSaving: boolean;
  runtimeSettingsMessage: string | null;
  saveRuntimeSettings: (keys?: string[]) => Promise<void> | void;
  resetRuntimeSetting: (key: string) => Promise<void> | void;
  canManageActions: boolean;
};

function recommendationBadgeStatus(state: string): string {
  switch (state) {
    case "enabled":
      return "enabled";
    case "manual":
      return "inactive";
    case "disabled_by_choice":
      return "disabled";
    case "recommended_login_required":
      return "pending";
    case "recommended_disabled":
      return "disabled";
    case "recommended_not_available":
      return "inactive";
    default:
      return "inactive";
  }
}

function serveBadgeStatus(status: string): string {
  switch (status) {
    case "configured":
      return "enabled";
    case "status_unavailable":
      return "bad";
    case "login_required":
      return "pending";
    case "off":
    case "not_installed":
    default:
      return "disabled";
  }
}

function serveLabel(status: string): string {
  switch (status) {
    case "configured":
      return "Active";
    case "status_unavailable":
      return "Unavailable";
    case "login_required":
      return "Login Required";
    case "off":
      return "Off";
    case "not_installed":
      return "Not Installed";
    default:
      return status.replace(/_/g, " ");
  }
}

function managementLabel(mode: string): string {
  return mode === "managed" ? "Managed by LabTether" : "Guided host setup";
}

function desiredModeLabel(mode: string): string {
  switch (mode) {
    case "serve":
      return "Tailscale HTTPS (Recommended)";
    case "manual":
      return "Manual / Existing TLS";
    case "off":
      return "Off For Now";
    default:
      return mode;
  }
}

type InfoRowProps = {
  title: string;
  description: string;
  value?: ReactNode;
  actions?: ReactNode;
};

function InfoRow({ title, description, value, actions }: InfoRowProps) {
  return (
    <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium text-[var(--text)]">{title}</span>
        <span className="text-xs text-[var(--muted)]">{description}</span>
      </div>
      <div className="flex min-w-0 flex-col items-start gap-2 sm:items-end">
        {value}
        {actions}
      </div>
    </div>
  );
}

export function TailscaleServeCard({
  copyToClipboard,
  copied,
  runtimeSettings,
  runtimeDraftValues,
  setRuntimeDraftValues,
  runtimeSettingsLoading,
  runtimeSettingsSaving,
  runtimeSettingsMessage,
  saveRuntimeSettings,
  resetRuntimeSetting,
  canManageActions,
}: TailscaleServeCardProps) {
  const { status, loading, error, refresh } = useTailscaleServeStatus();
  const [actionLoading, setActionLoading] = useState<"" | "apply" | "disable">("");
  const [actionMessage, setActionMessage] = useState("");
  const [actionError, setActionError] = useState("");

  if (!loading && error && !status) {
    return null;
  }

  const recommendationStatus = recommendationBadgeStatus(status?.recommendation_state ?? "");
  const settingsByKey = new Map(runtimeSettings.map((entry) => [entry.key, entry]));
  const modeEntry = settingsByKey.get(runtimeSettingKeys.remoteAccessMode);
  const targetEntry = settingsByKey.get(runtimeSettingKeys.remoteAccessTailscaleServeTarget);
  const draftMode = modeEntry ? (runtimeDraftValues[modeEntry.key] ?? modeEntry.effective_value) : (status?.desired_mode ?? "serve");
  const draftTarget = targetEntry ? (runtimeDraftValues[targetEntry.key] ?? targetEntry.effective_value) : (status?.desired_target ?? "");
  const tailscaleKeys = [runtimeSettingKeys.remoteAccessMode, runtimeSettingKeys.remoteAccessTailscaleServeTarget];
  const showRecommendationBanner = status && status.desired_mode === "serve" && status.recommendation_state !== "enabled";
  const managedActionsVisible = Boolean(status?.can_manage && canManageActions);

  const performManagedAction = async (action: "apply" | "disable") => {
    setActionLoading(action);
    setActionError("");
    setActionMessage("");
    try {
      const response = await fetch("/api/settings/tailscale/serve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action,
          target: action === "apply" ? draftTarget.trim() : undefined,
        }),
      });
      const payload = await response.json().catch(() => null) as { error?: string } | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setActionMessage(action === "apply" ? "Managed Tailscale HTTPS applied." : "Managed Tailscale HTTPS disabled.");
      refresh();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Tailscale HTTPS update failed");
    } finally {
      setActionLoading("");
    }
  };

  return (
    <Card className="mb-6">
      <div className="mb-3 flex items-center justify-between gap-3">
        <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">// Tailscale HTTPS</p>
        <Button
          variant="ghost"
          size="sm"
          loading={loading && Boolean(status)}
          onClick={refresh}
        >
          <RefreshCw size={13} className="shrink-0" />
          Verify
        </Button>
      </div>

      {loading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}

      {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}

      {!loading && !error && status ? (
        <div className="space-y-4">
          {showRecommendationBanner ? (
            <div className="flex items-start gap-2 rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/35 px-3 py-2.5">
              <Sparkles size={14} className="mt-0.5 shrink-0 text-[var(--accent)]" />
              <div className="space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge status={recommendationStatus} size="sm" />
                  <span className="text-xs font-medium text-[var(--text)]">Optional, but recommended</span>
                </div>
                <p className="text-xs text-[var(--text)]/85">{status.recommendation_message}</p>
              </div>
            </div>
          ) : null}

          <div className="divide-y divide-[var(--line)]">
            {modeEntry ? (
              <InfoRow
                title="Preferred Access Mode"
                description="Choose whether LabTether should keep recommending Tailscale HTTPS or stay out of the way."
                value={
                  <div className="flex w-full max-w-xs flex-col gap-2 sm:items-end">
                    <Select
                      className="w-full"
                      value={draftMode}
                      onChange={(event) =>
                        setRuntimeDraftValues((current) => ({ ...current, [modeEntry.key]: event.target.value }))
                      }
                      disabled={runtimeSettingsLoading}
                    >
                      <option value="serve">Tailscale HTTPS (Recommended)</option>
                      <option value="manual">Manual / Existing TLS</option>
                      <option value="off">Off For Now</option>
                    </Select>
                    <p className="text-right text-xs text-[var(--muted)]">
                      Current: {desiredModeLabel(status?.desired_mode ?? draftMode)} · Source: {status?.desired_mode_source ?? modeEntry.source}
                    </p>
                  </div>
                }
                actions={
                  <div className="flex flex-wrap gap-2 sm:justify-end">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => void resetRuntimeSetting(modeEntry.key)}
                    >
                      Reset
                    </Button>
                    <Button
                      variant="primary"
                      size="sm"
                      disabled={runtimeSettingsSaving || runtimeSettingsLoading}
                      onClick={() => void saveRuntimeSettings(tailscaleKeys)}
                    >
                      {runtimeSettingsSaving ? "Saving..." : "Save Mode"}
                    </Button>
                  </div>
                }
              />
            ) : null}

            {targetEntry ? (
              <InfoRow
                title="Preferred Serve Upstream"
                description="Optional host-local upstream URL for tailscale serve. Leave blank to follow LabTether's automatic localhost target."
                value={
                  <div className="flex w-full max-w-sm flex-col gap-2 sm:items-end">
                    <Input
                      className="w-full"
                      value={draftTarget}
                      placeholder={status?.suggested_target || "http://127.0.0.1:8080"}
                      onChange={(event) =>
                        setRuntimeDraftValues((current) => ({ ...current, [targetEntry.key]: event.target.value }))
                      }
                      disabled={runtimeSettingsLoading}
                    />
                    <p className="text-right text-xs text-[var(--muted)]">
                      {status?.desired_target
                        ? `Current override: ${status.desired_target}`
                        : `Auto target: ${status?.suggested_target || "Not available yet"}`}
                    </p>
                  </div>
                }
                actions={
                  <div className="flex flex-wrap gap-2 sm:justify-end">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => void resetRuntimeSetting(targetEntry.key)}
                    >
                      Reset
                    </Button>
                    <Button
                      variant="primary"
                      size="sm"
                      disabled={runtimeSettingsSaving || runtimeSettingsLoading}
                      onClick={() => void saveRuntimeSettings(tailscaleKeys)}
                    >
                      {runtimeSettingsSaving ? "Saving..." : "Save Target"}
                    </Button>
                  </div>
                }
              />
            ) : null}

            <InfoRow
              title="Access Mode"
              description="Tailscale HTTPS stays optional. This card reflects the current host state and your saved preference."
              value={<span className="text-sm text-[var(--text)]">{managementLabel(status.management_mode)}</span>}
            />

            <InfoRow
              title="Tailscale State"
              description="Whether the Docker host can be inspected for Tailscale status."
              value={
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Badge status={status.tailscale_installed ? (status.logged_in ? "enabled" : "pending") : "disabled"} size="sm" />
                  <span className="text-sm text-[var(--text)]">
                    {status.tailscale_installed
                      ? status.backend_state || (status.logged_in ? "Connected" : "Installed")
                      : "Not detected on host"}
                  </span>
                </div>
              }
            />

            {status.tailnet ? (
              <InfoRow
                title="Tailnet"
                description="The connected Tailscale network for this host."
                value={<span className="text-sm text-[var(--text)]">{status.tailnet}</span>}
              />
            ) : null}

            {status.tsnet_url ? (
              <InfoRow
                title="HTTPS URL"
                description="The secure tailnet URL users will open once Tailscale Serve is enabled."
                value={<code className="max-w-full break-all text-xs sm:text-sm">{status.tsnet_url}</code>}
                actions={
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => copyToClipboard(status.tsnet_url!, "tailscale-url", "Tailscale URL copied to clipboard")}
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "tailscale-url" ? "Copied" : "Copy URL"}
                  </Button>
                }
              />
            ) : null}

            <InfoRow
              title="Serve Status"
              description="Whether HTTPS proxying is already active for this host."
              value={
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Badge status={serveBadgeStatus(status.serve_status)} size="sm" />
                  <span className="text-sm text-[var(--text)]">{serveLabel(status.serve_status)}</span>
                </div>
              }
            />

            {managedActionsVisible ? (
              <InfoRow
                title="Managed Host Actions"
                description="Available only when this deployment explicitly grants LabTether host-side Tailscale control."
                actions={
                  <div className="flex flex-wrap gap-2 sm:justify-end">
                    <Button
                      variant="secondary"
                      size="sm"
                      loading={actionLoading === "apply"}
                      disabled={runtimeSettingsSaving || runtimeSettingsLoading}
                      onClick={() => void performManagedAction("apply")}
                    >
                      {status.serve_configured ? "Reapply HTTPS" : "Enable HTTPS"}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      loading={actionLoading === "disable"}
                      onClick={() => void performManagedAction("disable")}
                    >
                      Disable HTTPS
                    </Button>
                  </div>
                }
              />
            ) : null}

            {status.serve_target ? (
              <InfoRow
                title="Current Upstream"
                description="The local LabTether target currently behind Tailscale Serve."
                value={<code className="max-w-full break-all text-xs sm:text-sm">{status.serve_target}</code>}
              />
            ) : null}

            {status.suggested_target ? (
              <InfoRow
                title="Suggested Local Target"
                description="The localhost target LabTether expects Tailscale Serve to proxy on the Docker host."
                value={<code className="max-w-full break-all text-xs sm:text-sm">{status.suggested_target}</code>}
              />
            ) : null}

            {status.suggested_command ? (
              <InfoRow
                title="Suggested Host Command"
                description="Run this on the Docker host if you want the recommended manual path."
                value={<code className="max-w-full break-all text-xs sm:text-sm">{status.suggested_command}</code>}
                actions={
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      copyToClipboard(
                        status.suggested_command!,
                        "tailscale-command",
                        "Suggested Tailscale command copied to clipboard",
                      )
                    }
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "tailscale-command" ? "Copied" : "Copy Command"}
                  </Button>
                }
              />
            ) : null}

            {status.status_note ? (
              <InfoRow
                title="Status Detail"
                description="Extra host-side output that can help during setup or troubleshooting."
                value={<p className="max-w-[28rem] text-right text-xs text-[var(--muted)]">{status.status_note}</p>}
              />
            ) : null}
          </div>

          {runtimeSettingsMessage ? <p className="text-xs text-[var(--muted)]">{runtimeSettingsMessage}</p> : null}
          {actionMessage ? <p className="text-xs text-[var(--muted)]">{actionMessage}</p> : null}
          {actionError ? <p className="text-xs text-[var(--bad)]">{actionError}</p> : null}
        </div>
      ) : null}
    </Card>
  );
}
