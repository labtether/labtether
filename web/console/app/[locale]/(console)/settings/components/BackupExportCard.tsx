"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { apiFetch } from "../../../../lib/api";
import { downloadJSON } from "../../../../lib/export";

type SavedActionListResponse = {
  data?: unknown[];
  meta?: {
    total?: number;
    per_page?: number;
  };
};

export function BackupExportCard() {
  const t = useTranslations("settings");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function fetchAllSavedActions() {
    const pageSize = 100;
    let offset = 0;
    const items: unknown[] = [];

    while (true) {
      const result = await apiFetch<SavedActionListResponse>(`/api/v2/actions?limit=${pageSize}&offset=${offset}`);
      if (!result.response.ok) {
        return result;
      }

      const pageItems = Array.isArray(result.data?.data) ? result.data.data : [];
      items.push(...pageItems);

      const total = typeof result.data?.meta?.total === "number" ? result.data.meta.total : items.length;
      const perPage = typeof result.data?.meta?.per_page === "number" ? result.data.meta.per_page : pageSize;
      if (pageItems.length === 0 || items.length >= total || pageItems.length < perPage) {
        return {
          response: result.response,
          data: items,
        };
      }

      offset += perPage;
    }
  }

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
        savedActionsResult,
        alertRulesResult,
        notifChannelsResult,
      ] = await Promise.all([
        apiFetch("/api/v2/assets"),
        apiFetch("/api/groups"),
        apiFetch("/api/v2/webhooks"),
        apiFetch("/api/v2/schedules"),
        fetchAllSavedActions(),
        apiFetch("/alerts/rules"),
        apiFetch("/notifications/channels"),
      ]);

      const failedEndpoints: string[] = [];
      if (!assetsResult.response.ok) failedEndpoints.push("/api/v2/assets");
      if (!groupsResult.response.ok) failedEndpoints.push("/api/groups");
      if (!webhooksResult.response.ok) failedEndpoints.push("/api/v2/webhooks");
      if (!schedulesResult.response.ok) failedEndpoints.push("/api/v2/schedules");
      if (!savedActionsResult.response.ok) failedEndpoints.push("/api/v2/actions");
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
        actions: savedActionsResult.data,
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
