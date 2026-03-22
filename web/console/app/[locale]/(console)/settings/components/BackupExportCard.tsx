"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { apiFetch } from "../../../../lib/api";
import { downloadJSON } from "../../../../lib/export";

export function BackupExportCard() {
  const t = useTranslations("settings");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function handleExport() {
    setLoading(true);
    setMessage(null);
    setError(null);

    try {
      const [
        assetsResult,
        groupsResult,
        webhooksResult,
        schedulesResult,
        actionsResult,
        alertRulesResult,
        notifChannelsResult,
      ] = await Promise.all([
        apiFetch("/api/v2/assets"),
        apiFetch("/api/groups"),
        apiFetch("/api/v2/webhooks"),
        apiFetch("/api/v2/schedules"),
        apiFetch("/api/v2/actions"),
        apiFetch("/alerts/rules"),
        apiFetch("/notifications/channels"),
      ]);

      const failedEndpoints: string[] = [];
      if (!assetsResult.response.ok) failedEndpoints.push("/api/v2/assets");
      if (!groupsResult.response.ok) failedEndpoints.push("/api/groups");
      if (!webhooksResult.response.ok) failedEndpoints.push("/api/v2/webhooks");
      if (!schedulesResult.response.ok) failedEndpoints.push("/api/v2/schedules");
      if (!actionsResult.response.ok) failedEndpoints.push("/api/v2/actions");
      if (!alertRulesResult.response.ok) failedEndpoints.push("/alerts/rules");
      if (!notifChannelsResult.response.ok) failedEndpoints.push("/notifications/channels");

      if (failedEndpoints.length > 0) {
        throw new Error(`${t("backup.exportError")}: ${failedEndpoints.join(", ")}`);
      }

      const config = {
        exported_at: new Date().toISOString(),
        assets: assetsResult.data,
        groups: groupsResult.data,
        webhooks: webhooksResult.data,
        schedules: schedulesResult.data,
        actions: actionsResult.data,
        alert_rules: alertRulesResult.data,
        notification_channels: notifChannelsResult.data,
      };

      downloadJSON(config, `labtether-config-${new Date().toISOString().slice(0, 10)}.json`);
      setMessage(t("backup.exportSuccess"));
    } catch (err) {
      setError(err instanceof Error ? err.message : t("backup.exportError"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card className="mb-6">
      <h2>{t("backup.title")}</h2>
      <p className="text-sm text-[var(--muted)] mt-1 mb-4">
        {t("backup.description")}
      </p>

      <div className="flex items-center gap-3">
        <Button
          variant="secondary"
          loading={loading}
          onClick={() => void handleExport()}
        >
          {loading ? t("backup.exporting") : t("backup.exportButton")}
        </Button>

        {message && !error ? (
          <span className="text-xs text-[var(--ok)]">{message}</span>
        ) : null}

        {error ? (
          <span className="text-xs text-[var(--bad)]">{error}</span>
        ) : null}
      </div>
    </Card>
  );
}
