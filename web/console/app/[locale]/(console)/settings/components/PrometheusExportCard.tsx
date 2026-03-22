"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { SkeletonRow } from "../../../../components/ui/Skeleton";

// Setting keys (must match runtimesettings.Key* constants in Go)
const KEYS = {
  scrapeEnabled: "prometheus.scrape_enabled",
  remoteWriteEnabled: "prometheus.remote_write_enabled",
  remoteWriteURL: "prometheus.remote_write_url",
  remoteWriteUsername: "prometheus.remote_write_username",
  remoteWritePassword: "prometheus.remote_write_password",
  remoteWriteInterval: "prometheus.remote_write_interval",
  processMetricsEnabled: "prometheus.process_metrics_enabled",
  processMetricsTopN: "prometheus.process_metrics_top_n",
} as const;

type SettingsMap = Record<string, string>;

type TestState = "idle" | "testing" | "ok" | "error";

const SECTION_HEADING = "text-xs font-semibold uppercase tracking-wider text-[var(--muted)]";

const REMOTE_WRITE_INTERVALS = [
  { value: "10s", label: "10s" },
  { value: "30s", label: "30s" },
  { value: "1m", label: "1m" },
  { value: "5m", label: "5m" },
] as const;

const PROCESS_TOP_N_OPTIONS = [
  { value: "10", label: "10" },
  { value: "20", label: "20" },
  { value: "50", label: "50" },
] as const;

const REMOTE_WRITE_CREDENTIAL_KEYS = new Set([
  "prometheus.remote_write_url",
  "prometheus.remote_write_username",
  "prometheus.remote_write_password",
]);

function boolStr(value: string | undefined, fallback: boolean): boolean {
  if (value === "true") return true;
  if (value === "false") return false;
  return fallback;
}

function Toggle({
  checked,
  disabled,
  onChange,
  label,
}: {
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
  label: string;
}) {
  return (
    <label className="flex items-center gap-2 cursor-pointer select-none">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4 rounded border-[var(--line)] accent-[var(--accent)]"
      />
      <span className="text-sm text-[var(--text)]">{label}</span>
    </label>
  );
}

export function PrometheusExportCard() {
  const t = useTranslations("settings");
  const [settings, setSettings] = useState<SettingsMap>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Test-connection state
  const [testState, setTestState] = useState<TestState>("idle");
  const [testMessage, setTestMessage] = useState<string | null>(null);

  // Draft copy of mutable fields
  const [draft, setDraft] = useState<SettingsMap>({});

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch("/api/settings/runtime", { cache: "no-store" });
      const payload = await res.json().catch(() => null) as { settings?: Array<{ key: string; effective_value: string }> } | null;
      if (!res.ok) {
        throw new Error((payload as { error?: string })?.error ?? `load failed: ${res.status}`);
      }
      const map: SettingsMap = {};
      for (const entry of (payload?.settings ?? [])) {
        if (Object.values(KEYS).includes(entry.key as typeof KEYS[keyof typeof KEYS])) {
          map[entry.key] = entry.effective_value;
        }
      }
      setSettings(map);
      setDraft(map);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load prometheus settings");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const set = useCallback((key: string, value: string) => {
    setDraft((prev) => ({ ...prev, [key]: value }));
    if (REMOTE_WRITE_CREDENTIAL_KEYS.has(key)) {
      setTestState("idle");
      setTestMessage(null);
    }
  }, []);

  const save = useCallback(async () => {
    setSaving(true);
    setMessage(null);
    setError(null);
    try {
      const values: SettingsMap = {};
      for (const key of Object.values(KEYS)) {
        const next = (draft[key] ?? "").trim();
        if (next !== (settings[key] ?? "").trim()) {
          values[key] = next;
        }
      }
      if (Object.keys(values).length === 0) {
        setMessage(t("prometheus.noChanges"));
        return;
      }
      const res = await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ values }),
      });
      const payload = await res.json().catch(() => null) as { settings?: Array<{ key: string; effective_value: string }>; error?: string } | null;
      if (!res.ok) {
        throw new Error(payload?.error ?? `save failed: ${res.status}`);
      }
      const map: SettingsMap = {};
      for (const entry of (payload?.settings ?? [])) {
        if (Object.values(KEYS).includes(entry.key as typeof KEYS[keyof typeof KEYS])) {
          map[entry.key] = entry.effective_value;
        }
      }
      setSettings(map);
      setDraft(map);
      setMessage(t("prometheus.saved"));
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to save prometheus settings");
    } finally {
      setSaving(false);
    }
  }, [t, draft, settings]);

  const testConnection = useCallback(async () => {
    setTestState("testing");
    setTestMessage(null);
    try {
      const res = await fetch("/api/settings/prometheus/test-connection", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          url: (draft[KEYS.remoteWriteURL] ?? "").trim(),
          username: (draft[KEYS.remoteWriteUsername] ?? "").trim(),
          password: (draft[KEYS.remoteWritePassword] ?? "").trim(),
        }),
      });
      const payload = await res.json().catch(() => null) as { success?: boolean; error?: string } | null;
      if (!res.ok) {
        setTestState("error");
        setTestMessage(payload?.error ?? `test failed: ${res.status}`);
        return;
      }
      if (payload?.success) {
        setTestState("ok");
        setTestMessage(t("prometheus.connectionSucceeded"));
      } else {
        setTestState("error");
        setTestMessage(payload?.error ?? t("prometheus.connectionFailed"));
      }
    } catch (err) {
      setTestState("error");
      setTestMessage(err instanceof Error ? err.message : "Connection test failed.");
    }
  }, [t, draft]);

  // Convenience reads from draft
  const scrapeEnabled = boolStr(draft[KEYS.scrapeEnabled], false);
  const remoteWriteEnabled = boolStr(draft[KEYS.remoteWriteEnabled], false);
  const processMetricsEnabled = boolStr(draft[KEYS.processMetricsEnabled], false);

  const scrapeURL = `http://<hub-host>:8080/metrics`;

  return (
    <Card className="mb-6">
      <h2>{t("prometheus.title")}</h2>
      <p className="text-sm text-[var(--muted)] mt-1 mb-4">
        {t("prometheus.description")}
      </p>

      {loading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : (
        <div className="space-y-6">
          {/* Prometheus Scrape */}
          <div>
            <p className={SECTION_HEADING}>{t("prometheus.scrapeEndpoint")}</p>
            <div className="mt-3 space-y-3">
              <Toggle
                label={t("prometheus.enableScrape")}
                checked={scrapeEnabled}
                disabled={saving}
                onChange={(v) => set(KEYS.scrapeEnabled, String(v))}
              />
              {scrapeEnabled && (
                <div className="flex items-center gap-2">
                  <code className="flex-1 rounded border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)] font-mono select-all">
                    {scrapeURL}
                  </code>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void navigator.clipboard.writeText(scrapeURL)}
                  >
                    {t("prometheus.copy")}
                  </Button>
                </div>
              )}
              <p className="text-xs text-[var(--muted)]">
                {t("prometheus.scrapeDescription")}
              </p>
            </div>
          </div>

          {/* Remote Write */}
          <div>
            <p className={SECTION_HEADING}>{t("prometheus.remoteWrite")}</p>
            <div className="mt-3 space-y-3">
              <Toggle
                label={t("prometheus.enableRemoteWrite")}
                checked={remoteWriteEnabled}
                disabled={saving}
                onChange={(v) => set(KEYS.remoteWriteEnabled, String(v))}
              />
              {remoteWriteEnabled && (
                <div className="grid gap-3 md:grid-cols-2">
                  <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)] md:col-span-2">
                    {t("prometheus.remoteWriteURL")}
                    <Input
                      value={draft[KEYS.remoteWriteURL] ?? ""}
                      onChange={(e) => set(KEYS.remoteWriteURL, e.target.value)}
                      placeholder="https://prometheus.example.com/api/v1/write"
                      disabled={saving}
                    />
                  </label>
                  <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                    {t("prometheus.username")} <span className="text-[var(--muted)]/60">{t("prometheus.optional")}</span>
                    <Input
                      value={draft[KEYS.remoteWriteUsername] ?? ""}
                      onChange={(e) => set(KEYS.remoteWriteUsername, e.target.value)}
                      placeholder="e.g. prometheus"
                      disabled={saving}
                    />
                  </label>
                  <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                    {t("prometheus.password")} <span className="text-[var(--muted)]/60">{t("prometheus.optional")}</span>
                    <Input
                      type="password"
                      value={draft[KEYS.remoteWritePassword] ?? ""}
                      onChange={(e) => set(KEYS.remoteWritePassword, e.target.value)}
                      placeholder={t("prometheus.passwordPlaceholder")}
                      disabled={saving}
                    />
                  </label>
                  <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)]">
                    {t("prometheus.pushInterval")}
                    <Select
                      value={draft[KEYS.remoteWriteInterval] ?? "30s"}
                      onChange={(e) => set(KEYS.remoteWriteInterval, e.target.value)}
                      disabled={saving}
                    >
                      {REMOTE_WRITE_INTERVALS.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <div className="flex items-end gap-2">
                    <Button
                      variant="secondary"
                      size="sm"
                      loading={testState === "testing"}
                      disabled={saving || !(draft[KEYS.remoteWriteURL] ?? "").trim()}
                      onClick={() => void testConnection()}
                    >
                      {t("prometheus.testConnection")}
                    </Button>
                    {testState === "ok" && (
                      <span className="text-xs text-[var(--good)]">{testMessage}</span>
                    )}
                    {testState === "error" && (
                      <span className="text-xs text-[var(--bad)]">{testMessage}</span>
                    )}
                  </div>
                </div>
              )}
              <p className="text-xs text-[var(--muted)]">
                {t("prometheus.remoteWriteDescription")}
              </p>
            </div>
          </div>

          {/* Process Metrics */}
          <div>
            <p className={SECTION_HEADING}>{t("prometheus.processMetrics")}</p>
            <div className="mt-3 space-y-3">
              <Toggle
                label={t("prometheus.enableProcessMetrics")}
                checked={processMetricsEnabled}
                disabled={saving}
                onChange={(v) => set(KEYS.processMetricsEnabled, String(v))}
              />
              {processMetricsEnabled && (
                <label className="flex flex-col gap-1.5 text-xs text-[var(--muted)] w-32">
                  {t("prometheus.topNProcesses")}
                  <Select
                    value={draft[KEYS.processMetricsTopN] ?? "10"}
                    onChange={(e) => set(KEYS.processMetricsTopN, e.target.value)}
                    disabled={saving}
                  >
                    {PROCESS_TOP_N_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </Select>
                </label>
              )}
              <p className="text-xs text-[var(--muted)]">
                {t("prometheus.processMetricsDescription")}
              </p>
            </div>
          </div>
        </div>
      )}

      {error ? <p className="mt-3 text-sm text-[var(--bad)]">{error}</p> : null}

      <div className="flex items-center gap-3 pt-4">
        <Button variant="primary" loading={saving} disabled={loading} onClick={() => void save()}>
          {t("prometheus.saveSettings")}
        </Button>
        {message ? <span className="text-xs text-[var(--muted)]">{message}</span> : null}
      </div>
    </Card>
  );
}
