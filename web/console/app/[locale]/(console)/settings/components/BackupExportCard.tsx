"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { apiFetch } from "../../../../lib/api";
import { downloadJSON } from "../../../../lib/export";
import { fetchAllSchedules } from "../../../../lib/schedules";

type SavedActionListResponse = {
  data?: Array<{ id: string }>;
  meta?: {
    total?: number;
    page?: number;
    per_page?: number;
  };
};

const CONFIG_PAGE_SIZE = 100;
const MAX_CONFIG_PAGES = 100;

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isCompleteAssetList(value: unknown): boolean {
  if (!isRecord(value) || !Array.isArray(value.data) || !isRecord(value.meta)) return false;
  const { total, page, per_page: perPage } = value.meta;
  return typeof total === "number" && Number.isSafeInteger(total)
    && total === value.data.length && page === 1 && perPage === total;
}

async function fetchAllOffsetRows(path: string, key: "rules" | "channels") {
  const items: Array<{ id: string }> = [];
  const seenIDs = new Set<string>();
  let firstPayload: Record<string, unknown> | null = null;

  for (let page = 0; page < MAX_CONFIG_PAGES; page++) {
    const offset = page * CONFIG_PAGE_SIZE;
    const { response, data } = await apiFetch<Record<string, unknown>>(
      `${path}?limit=${CONFIG_PAGE_SIZE}&offset=${offset}`,
    );
    if (!response.ok) {
      throw new Error(`Failed to load ${path} (offset ${offset}, HTTP ${response.status}).`);
    }

    const pageItems = data?.[key];
    if (!Array.isArray(pageItems) || pageItems.length > CONFIG_PAGE_SIZE) {
      throw new Error(`Incomplete ${path} response (offset ${offset}).`);
    }
    for (const item of pageItems) {
      if (!item || typeof item.id !== "string" || !item.id.trim() || seenIDs.has(item.id)) {
        throw new Error(`Incomplete ${path} response (offset ${offset}).`);
      }
      seenIDs.add(item.id);
      items.push(item);
    }

    firstPayload ??= data;
    if (pageItems.length < CONFIG_PAGE_SIZE) {
      return { ...firstPayload, [key]: items };
    }
  }

  throw new Error(`${path} exceeds the supported export page limit.`);
}

export function BackupExportCard() {
  const t = useTranslations("settings");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function fetchAllSavedActions() {
    const items: Array<{ id: string }> = [];
    const seenIDs = new Set<string>();
    let expectedTotal: number | null = null;

    for (let page = 1; page <= MAX_CONFIG_PAGES; page++) {
      const offset = (page - 1) * CONFIG_PAGE_SIZE;
      const { response, data } = await apiFetch<SavedActionListResponse>(
        `/api/v2/actions?limit=${CONFIG_PAGE_SIZE}&offset=${offset}`,
      );
      if (!response.ok) {
        throw new Error(`Failed to load saved actions (offset ${offset}, HTTP ${response.status}).`);
      }

      const total = data?.meta?.total;
      const pageItems = data?.data;
      if (
        !Array.isArray(pageItems)
        || typeof total !== "number"
        || !Number.isSafeInteger(total)
        || total < 0
        || total > MAX_CONFIG_PAGES * CONFIG_PAGE_SIZE
        || data?.meta?.page !== page
        || data?.meta?.per_page !== CONFIG_PAGE_SIZE
        || (expectedTotal !== null && total !== expectedTotal)
        || pageItems.length !== Math.min(CONFIG_PAGE_SIZE, total - items.length)
      ) {
        throw new Error(`Incomplete saved actions response (offset ${offset}).`);
      }
      for (const item of pageItems) {
        if (!item || typeof item.id !== "string" || !item.id.trim() || seenIDs.has(item.id)) {
          throw new Error(`Incomplete saved actions response (offset ${offset}).`);
        }
        seenIDs.add(item.id);
      }

      expectedTotal = total;
      items.push(...pageItems);
      if (items.length === total) return items;
    }

    throw new Error("Saved actions exceed the supported export page limit.");
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
        fetchAllSchedules(),
        fetchAllSavedActions(),
        fetchAllOffsetRows("/api/alerts/rules", "rules"),
        fetchAllOffsetRows("/api/notifications/channels", "channels"),
      ]);

      const failedEndpoints: string[] = [];
      if (!assetsResult.response.ok) failedEndpoints.push("/api/v2/assets");
      if (!groupsResult.response.ok) failedEndpoints.push("/api/groups");
      if (!webhooksResult.response.ok) failedEndpoints.push("/api/v2/webhooks");

      if (failedEndpoints.length > 0) {
        throw new Error(`${t("backup.exportError")}: ${failedEndpoints.join(", ")}`);
      }
      if (
        !isCompleteAssetList(assetsResult.data)
        || !isRecord(groupsResult.data)
        || !Array.isArray(groupsResult.data.groups)
        || !isRecord(webhooksResult.data)
        || !Array.isArray(webhooksResult.data.data)
      ) {
        throw new Error("Incomplete inventory or webhook response.");
      }

      const config = {
        exported_at: new Date().toISOString(),
        assets: assetsResult.data,
        groups: groupsResult.data,
        webhooks: webhooksResult.data,
        schedules: schedulesResult,
        actions: savedActionsResult,
        alert_rules: alertRulesResult,
        notification_channels: notifChannelsResult,
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
