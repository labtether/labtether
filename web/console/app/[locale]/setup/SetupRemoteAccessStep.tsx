"use client";

import { useTranslations } from "next-intl";
import { CheckCircle2, Copy, RefreshCw, Shield, SkipForward } from "lucide-react";
import { Button } from "../../components/ui/Button";
import { Badge } from "../../components/ui/Badge";
import type { useTailscaleServeStatus } from "../../hooks/useTailscaleServeStatus";

export type SetupRemoteAccessStepProps = {
  tailscaleStatus: ReturnType<typeof useTailscaleServeStatus>["status"];
  tailscaleLoading: boolean;
  tailscaleError: ReturnType<typeof useTailscaleServeStatus>["error"];
  refreshTailscaleStatus: () => void;
  copied: string;
  copyToClipboard: (text: string, label: string) => void;
  managedActionLoading: "" | "apply" | "disable";
  managedActionMessage: string;
  managedActionError: string;
  runManagedTailscaleAction: (action: "apply" | "disable") => Promise<void>;
  continueToConsole: (mode: "serve" | "off") => Promise<void>;
};

export function SetupRemoteAccessStep({
  tailscaleStatus,
  tailscaleLoading,
  tailscaleError,
  refreshTailscaleStatus,
  copied,
  copyToClipboard,
  managedActionLoading,
  managedActionMessage,
  managedActionError,
  runManagedTailscaleAction,
  continueToConsole,
}: SetupRemoteAccessStepProps) {
  const t = useTranslations("auth");
  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/30 px-4 py-3">
        <div className="flex items-start gap-3">
          <Shield
            size={16}
            className="mt-0.5 shrink-0 text-[var(--accent)]"
          />
          <div className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <Badge
                status={
                  tailscaleStatus?.serve_configured
                    ? "enabled"
                    : "pending"
                }
                size="sm"
              />
              <span className="text-sm font-medium text-[var(--text)]">
                {t("remoteAccess.recommended")}
              </span>
            </div>
            <p className="text-sm text-[var(--text)]/90">
              {t("remoteAccess.description")}
            </p>
            <p className="text-xs text-[var(--muted)]">
              {t("remoteAccess.skipNote")}
            </p>
          </div>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
        <div className="space-y-4">
          <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-4">
            <p className="text-[10px] font-mono uppercase tracking-[0.14em] text-[var(--muted)]">
              {t("remoteAccess.suggestedSteps")}
            </p>
            <div className="mt-3 space-y-3 text-sm text-[var(--text)]">
              <div>
                <p className="font-medium">
                  {t("remoteAccess.step1.title")}
                </p>
                <p className="text-xs text-[var(--muted)]">
                  {t("remoteAccess.step1.description")}
                </p>
              </div>
              <div>
                <p className="font-medium">
                  {t("remoteAccess.step2.title")}
                </p>
                {tailscaleStatus?.suggested_command ? (
                  <div className="mt-2 rounded-lg border border-[var(--line)] bg-[var(--bg)]/70 p-3">
                    <code className="block break-all text-xs sm:text-sm">
                      {tailscaleStatus.suggested_command}
                    </code>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() =>
                          copyToClipboard(
                            tailscaleStatus.suggested_command!,
                            "setup-command",
                          )
                        }
                      >
                        <Copy size={13} className="shrink-0" />
                        {copied === "setup-command"
                          ? t("remoteAccess.copied")
                          : t("remoteAccess.copyCommand")}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <p className="mt-1 text-xs text-[var(--muted)]">
                    {t("remoteAccess.step2.fallback")}
                  </p>
                )}
              </div>
              <div>
                <p className="font-medium">
                  {t("remoteAccess.step3.title")}
                </p>
                <p className="text-xs text-[var(--muted)]">
                  {t("remoteAccess.step3.description")}
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p className="text-[10px] font-mono uppercase tracking-[0.14em] text-[var(--muted)]">
                  {t("remoteAccess.detectedStatus")}
                </p>
                <p className="mt-1 text-sm text-[var(--text)]">
                  {t("remoteAccess.detectedDescription")}
                </p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                loading={tailscaleLoading && Boolean(tailscaleStatus)}
                onClick={refreshTailscaleStatus}
              >
                <RefreshCw size={13} className="shrink-0" />
                {t("remoteAccess.verify")}
              </Button>
            </div>

            {tailscaleError && !tailscaleStatus ? (
              <div className="mt-3 rounded-lg bg-[var(--bad-glow)] px-3 py-2 text-sm text-[var(--bad)]">
                {tailscaleError}
              </div>
            ) : null}

            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <div className="rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                <p className="text-[10px] font-mono uppercase tracking-[0.12em] text-[var(--muted)]">
                  {t("remoteAccess.tailscale")}
                </p>
                <div className="mt-2 flex items-center gap-2">
                  <Badge
                    status={
                      tailscaleStatus?.tailscale_installed
                        ? tailscaleStatus.logged_in
                          ? "enabled"
                          : "pending"
                        : "disabled"
                    }
                    size="sm"
                  />
                  <span className="text-sm text-[var(--text)]">
                    {tailscaleStatus
                      ? tailscaleStatus.tailscale_installed
                        ? tailscaleStatus.backend_state ||
                          (tailscaleStatus.logged_in
                            ? t("remoteAccess.connected")
                            : t("remoteAccess.installed"))
                        : t("remoteAccess.notDetected")
                      : tailscaleLoading
                        ? t("remoteAccess.checking")
                        : t("remoteAccess.unavailable")}
                  </span>
                </div>
              </div>

              <div className="rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                <p className="text-[10px] font-mono uppercase tracking-[0.12em] text-[var(--muted)]">
                  {t("remoteAccess.serve")}
                </p>
                <div className="mt-2 flex items-center gap-2">
                  <Badge
                    status={
                      tailscaleStatus?.serve_configured
                        ? "enabled"
                        : tailscaleStatus?.logged_in
                          ? "pending"
                          : "disabled"
                    }
                    size="sm"
                  />
                  <span className="text-sm text-[var(--text)]">
                    {tailscaleStatus
                      ? tailscaleStatus.serve_configured
                        ? t("remoteAccess.httpsActive")
                        : tailscaleStatus.serve_status ===
                            "login_required"
                          ? t("remoteAccess.loginRequired")
                          : tailscaleStatus.serve_status ===
                              "not_installed"
                            ? t("remoteAccess.notInstalled")
                            : t("remoteAccess.notConfigured")
                      : tailscaleLoading
                        ? t("remoteAccess.checking")
                        : t("remoteAccess.unavailable")}
                  </span>
                </div>
              </div>
            </div>

            {tailscaleStatus?.tsnet_url ? (
              <div className="mt-3 rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
                <p className="text-[10px] font-mono uppercase tracking-[0.12em] text-[var(--muted)]">
                  {t("remoteAccess.httpsUrl")}
                </p>
                <code className="mt-2 block break-all text-xs sm:text-sm">
                  {tailscaleStatus.tsnet_url}
                </code>
                <div className="mt-3">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      copyToClipboard(
                        tailscaleStatus.tsnet_url!,
                        "setup-url",
                      )
                    }
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "setup-url"
                      ? t("remoteAccess.copied")
                      : t("remoteAccess.copyUrl")}
                  </Button>
                </div>
              </div>
            ) : null}

            {tailscaleStatus?.status_note ? (
              <p className="mt-3 text-xs text-[var(--muted)]">
                {tailscaleStatus.status_note}
              </p>
            ) : null}

            {tailscaleStatus?.can_manage ? (
              <div className="mt-4 rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/15 px-3 py-3">
                <p className="text-sm font-medium text-[var(--text)]">
                  {t("remoteAccess.managedTitle")}
                </p>
                <p className="mt-1 text-xs text-[var(--muted)]">
                  {t("remoteAccess.managedDescription")}
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button
                    variant="primary"
                    size="sm"
                    loading={managedActionLoading === "apply"}
                    onClick={() =>
                      void runManagedTailscaleAction("apply")
                    }
                  >
                    {tailscaleStatus.serve_configured
                      ? t("remoteAccess.reapplyHttps")
                      : t("remoteAccess.enableHttps")}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    loading={managedActionLoading === "disable"}
                    onClick={() =>
                      void runManagedTailscaleAction("disable")
                    }
                  >
                    {t("remoteAccess.disableHttps")}
                  </Button>
                </div>
                {managedActionMessage ? (
                  <p className="mt-3 text-xs text-[var(--muted)]">
                    {managedActionMessage}
                  </p>
                ) : null}
                {managedActionError ? (
                  <p className="mt-3 text-xs text-[var(--bad)]">
                    {managedActionError}
                  </p>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>

        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)]/55 p-4">
          <p className="text-[10px] font-mono uppercase tracking-[0.14em] text-[var(--muted)]">
            {t("remoteAccess.finishSetup")}
          </p>
          <div className="mt-3 space-y-3">
            <div className="rounded-lg border border-[var(--line)] bg-[var(--bg)]/60 px-3 py-3">
              <p className="text-sm font-medium text-[var(--text)]">
                {t("remoteAccess.continueNow")}
              </p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                {t("remoteAccess.continueNowDescription")}
              </p>
              <Button
                variant="secondary"
                className="mt-3 w-full"
                onClick={() => void continueToConsole("off")}
              >
                <SkipForward size={14} className="shrink-0" />
                {t("remoteAccess.continueWithout")}
              </Button>
            </div>

            <div className="rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/20 px-3 py-3">
              <p className="text-sm font-medium text-[var(--text)]">
                {t("remoteAccess.finishRecommended")}
              </p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                {t("remoteAccess.finishRecommendedDescription")}
              </p>
              <Button
                variant="primary"
                className="mt-3 w-full"
                onClick={() => void continueToConsole("serve")}
              >
                <CheckCircle2 size={14} className="shrink-0" />
                {t("remoteAccess.openLabTether")}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
