"use client";

import type { Dispatch, SetStateAction } from "react";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { runtimeSettingKeys, type RuntimeSettingEntry } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { useTailscaleServeStatus, type TailscaleServeStatus } from "../../../../hooks/useTailscaleServeStatus";
import { CodeBlock, CopyButton, Facts, Field, Message, Row, Status } from "./SettingsKit";

type TailscaleServeCardProps = {
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

const SERVE_TONES: Record<string, "ok" | "warn" | "bad" | "off"> = {
  configured: "ok", login_required: "warn", status_unavailable: "bad", off: "off", not_installed: "off",
};
const SERVE_KEYS: Record<string, string> = {
  configured: "serving", login_required: "loginRequired", status_unavailable: "unavailable", off: "off", not_installed: "notInstalled",
};

/** The key for the host's Tailscale state when the hub reports no backend state of its own. */
function tailscaleStateKey(status: TailscaleServeStatus): string {
  if (!status.tailscale_installed) return "ts.notDetected";
  return status.logged_in ? "ts.connected" : "ts.installed";
}

/** The Tailscale Serve row of Security: its state and address, then the saved preference and host facts behind Show. */
export function TailscaleServeCard({
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
  const t = useTranslations("security");
  const { status, loading, error, refresh } = useTailscaleServeStatus();
  const [open, setOpen] = useState(false);
  const [actionLoading, setActionLoading] = useState<"" | "apply" | "disable">("");
  const [actionMessage, setActionMessage] = useState("");
  const [actionError, setActionError] = useState("");

  if (!loading && error && !status) return null;

  const settingsByKey = new Map(runtimeSettings.map((entry) => [entry.key, entry]));
  const modeEntry = settingsByKey.get(runtimeSettingKeys.remoteAccessMode);
  const targetEntry = settingsByKey.get(runtimeSettingKeys.remoteAccessTailscaleServeTarget);
  const draftMode = modeEntry ? (runtimeDraftValues[modeEntry.key] ?? modeEntry.effective_value) : (status?.desired_mode ?? "serve");
  const draftTarget = targetEntry ? (runtimeDraftValues[targetEntry.key] ?? targetEntry.effective_value) : (status?.desired_target ?? "");
  const tailscaleKeys = [runtimeSettingKeys.remoteAccessMode, runtimeSettingKeys.remoteAccessTailscaleServeTarget];
  const preferenceDirty = Boolean(
    (modeEntry && draftMode !== modeEntry.effective_value) || (targetEntry && draftTarget.trim() !== targetEntry.effective_value),
  );
  const canReset = modeEntry?.source === "ui" || targetEntry?.source === "ui";

  const performManagedAction = async (action: "apply" | "disable") => {
    setActionLoading(action);
    setActionError("");
    setActionMessage("");
    try {
      const response = await fetch("/api/settings/tailscale/serve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, target: action === "apply" ? draftTarget.trim() : undefined }),
      });
      const payload = await response.json().catch(() => null) as { error?: string } | null;
      if (!response.ok) throw new Error(payload?.error || t("ts.failed"));
      setActionMessage(action === "apply" ? t("ts.applied") : t("ts.turnedOff"));
      refresh();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : t("ts.failed"));
    } finally {
      setActionLoading("");
    }
  };

  const serveStatus = status?.serve_status ?? "";
  const summary = status ? (
    <>
      {serveStatus === "configured" ? null : <Status tone={SERVE_TONES[serveStatus] ?? "off"}>{t(`ts.status.${SERVE_KEYS[serveStatus] ?? "off"}`)}</Status>}
      {status.tsnet_url ? (
        <span className="st-cv">
          <span>{status.tsnet_url.replace(/^https:\/\//u, "")}</span>
          <CopyButton text={status.tsnet_url} label={t("ts.copyUrl")} />
        </span>
      ) : null}
    </>
  ) : null;

  return (
    <>
      <Row label={t("ts.title")} hint={summary}>
        <Button size="sm" onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls="st-ts-details" disabled={!status}>
          {open ? t("hide") : t("show")}
        </Button>
      </Row>

      {open && status ? (
        <div className="st-x" id="st-ts-details">
          {status.desired_mode === "serve" && status.recommendation_state !== "enabled" && status.recommendation_message ? (
            <p className="st-g">{status.recommendation_message}</p>
          ) : null}

          {modeEntry || targetEntry ? (
            <div className="st-form">
              {modeEntry ? (
                <Field label={t("ts.mode")} htmlFor="st-ts-mode">
                  <Select
                    id="st-ts-mode"
                    className="w-full"
                    value={draftMode}
                    onChange={(event) => setRuntimeDraftValues((current) => ({ ...current, [modeEntry.key]: event.target.value }))}
                    disabled={runtimeSettingsLoading}
                  >
                    <option value="serve">{t("ts.modeServe")}</option>
                    <option value="manual">{t("ts.modeManual")}</option>
                    <option value="off">{t("ts.modeOff")}</option>
                  </Select>
                </Field>
              ) : null}
              {targetEntry ? (
                <Field
                  label={t("ts.upstream")}
                  htmlFor="st-ts-upstream"
                  hint={status.desired_target ? undefined : t("ts.upstreamAuto", { target: status.suggested_target || "—" })}
                >
                  <Input
                    id="st-ts-upstream"
                    value={draftTarget}
                    placeholder={status.suggested_target || "http://127.0.0.1:8080"}
                    onChange={(event) => setRuntimeDraftValues((current) => ({ ...current, [targetEntry.key]: event.target.value }))}
                    disabled={runtimeSettingsLoading}
                  />
                </Field>
              ) : null}
              <div className="st-acts st-field wide">
                <Button size="sm" disabled={runtimeSettingsSaving || runtimeSettingsLoading || !preferenceDirty} onClick={() => void saveRuntimeSettings(tailscaleKeys)}>
                  {runtimeSettingsSaving ? t("saving") : t("save")}
                </Button>
                {canReset ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={runtimeSettingsSaving}
                    onClick={() => {
                      if (modeEntry?.source === "ui") void resetRuntimeSetting(modeEntry.key);
                      if (targetEntry?.source === "ui") void resetRuntimeSetting(targetEntry.key);
                    }}
                  >
                    {t("ts.reset")}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}

          <Facts
            items={[
              [t("ts.management"), status.management_mode === "managed" ? t("ts.managed") : t("ts.guided")],
              [t("ts.state"), status.tailscale_installed && status.backend_state ? status.backend_state : t(tailscaleStateKey(status))],
              status.tailnet ? [t("ts.tailnet"), status.tailnet] : false,
              [t("ts.serve"), t(`ts.status.${SERVE_KEYS[serveStatus] ?? "off"}`)],
              status.serve_target ? [t("ts.currentUpstream"), <code key="up" className="st-mono">{status.serve_target}</code>] : false,
              status.suggested_target ? [t("ts.suggestedTarget"), <code key="sg" className="st-mono">{status.suggested_target}</code>] : false,
              status.status_note ? [t("ts.note"), <span key="nt" className="st-g">{status.status_note}</span>] : false,
            ]}
          />

          {status.suggested_command ? <CodeBlock code={status.suggested_command} copyLabel={t("ts.copyCommand")} /> : null}

          <div className="st-acts">
            {status.can_manage && canManageActions ? (
              <>
                <Button size="sm" loading={actionLoading === "apply"} disabled={runtimeSettingsSaving || runtimeSettingsLoading} onClick={() => void performManagedAction("apply")}>
                  {status.serve_configured ? t("ts.reapply") : t("ts.enable")}
                </Button>
                <Button size="sm" variant="ghost" loading={actionLoading === "disable"} onClick={() => void performManagedAction("disable")}>
                  {t("ts.disable")}
                </Button>
              </>
            ) : null}
            <Button size="sm" variant="ghost" loading={loading} onClick={refresh}>{t("ts.verify")}</Button>
          </div>
        </div>
      ) : null}

      {runtimeSettingsMessage && open ? <Message>{runtimeSettingsMessage}</Message> : null}
      {actionMessage ? <Message>{actionMessage}</Message> : null}
      {actionError ? <Message tone="bad">{actionError}</Message> : null}
      {error && status ? <Message tone="bad">{error}</Message> : null}
    </>
  );
}
