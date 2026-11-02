"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import {
  PROMETHEUS_SETTING_KEYS as KEYS,
  buildPrometheusPatch,
  buildPrometheusSettingsState,
  buildPrometheusTestRequest,
  type PrometheusSettingsMap as SettingsMap,
} from "./prometheusSettings";
import { CopyButton, Field, Row, Section, Sheet, SheetFoot, Switch } from "./SettingsKit";

type TestState = "idle" | "testing" | "ok" | "error";

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

/** Whether the hub serves HTTPS (Security › HTTPS); true until the hub says otherwise, as TLS is on by default. */
function useHubTLSEnabled(): boolean {
  const [enabled, setEnabled] = useState(true);
  useEffect(() => {
    let live = true;
    fetch("/api/settings/tls", { cache: "no-store" })
      .then((response) => (response.ok ? response.json() : null))
      .then((payload: { tls_enabled?: unknown } | null) => {
        if (live && typeof payload?.tls_enabled === "boolean") setEnabled(payload.tls_enabled);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);
  return enabled;
}

function boolStr(value: string | undefined, fallback: boolean): boolean {
  if (value === "true") return true;
  if (value === "false") return false;
  return fallback;
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
  const [passwordConfigured, setPasswordConfigured] = useState(false);

  // Draft copy of mutable fields
  const [draft, setDraft] = useState<SettingsMap>({});
  const tlsOn = useHubTLSEnabled();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch("/api/settings/runtime", { cache: "no-store" });
      const payload = await res.json().catch(() => null) as { settings?: Array<{ key: string; effective_value?: string; sensitive?: boolean; configured?: boolean }> } | null;
      if (!res.ok) {
        throw new Error((payload as { error?: string })?.error ?? `load failed: ${res.status}`);
      }
      const state = buildPrometheusSettingsState(payload?.settings ?? []);
      setSettings(state.values);
      setDraft(state.values);
      setPasswordConfigured(state.passwordConfigured);
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
      const values = buildPrometheusPatch(draft, settings);
      if (Object.keys(values).length === 0) {
        setMessage(t("prometheus.noChanges"));
        return;
      }
      const res = await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ values }),
      });
      const payload = await res.json().catch(() => null) as { settings?: Array<{ key: string; effective_value?: string; sensitive?: boolean; configured?: boolean }>; error?: string } | null;
      if (!res.ok) {
        throw new Error(payload?.error ?? `save failed: ${res.status}`);
      }
      const state = buildPrometheusSettingsState(payload?.settings ?? []);
      setSettings(state.values);
      setDraft(state.values);
      setPasswordConfigured(state.passwordConfigured);
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
        body: JSON.stringify(buildPrometheusTestRequest(draft, passwordConfigured)),
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
      setTestMessage(t("prometheus.connectionFailed"));
    }
  }, [t, draft, passwordConfigured]);

  // Convenience reads from draft
  const scrapeEnabled = boolStr(draft[KEYS.scrapeEnabled], false);
  const remoteWriteEnabled = boolStr(draft[KEYS.remoteWriteEnabled], false);
  const processMetricsEnabled = boolStr(draft[KEYS.processMetricsEnabled], false);

  // With hub TLS on (the default) :8080 only redirects, so a scraper must use HTTPS on 8443.
  const scrapeURL = tlsOn ? "https://<hub-host>:8443/metrics" : "http://<hub-host>:8080/metrics";
  const dirty = Object.keys(buildPrometheusPatch(draft, settings)).length > 0;

  return (
    <Section title={t("prometheus.title")} id="prometheus">
      <Sheet className={loading ? "st-loading" : ""}>
        {loading ? null : (
          <>
            <Row
              label={t("prometheus.scrapeEndpoint")}
              hint={scrapeEnabled ? <span className="st-cv"><code className="st-mono">{scrapeURL}</code><CopyButton text={scrapeURL} label={t("prometheus.copy")} /></span> : null}
              className="st-sw-row"
            >
              <Switch checked={scrapeEnabled} disabled={saving} onChange={(v) => set(KEYS.scrapeEnabled, String(v))} label={t("prometheus.enableScrape")} />
            </Row>

            <Row label={t("prometheus.remoteWrite")} className="st-sw-row">
              <Switch checked={remoteWriteEnabled} disabled={saving} onChange={(v) => set(KEYS.remoteWriteEnabled, String(v))} label={t("prometheus.enableRemoteWrite")} />
            </Row>
            {remoteWriteEnabled ? (
              <div className="st-form">
                <Field label={t("prometheus.remoteWriteURL")} htmlFor="st-prom-url" wide>
                  <Input
                    id="st-prom-url"
                    value={draft[KEYS.remoteWriteURL] ?? ""}
                    onChange={(e) => set(KEYS.remoteWriteURL, e.target.value)}
                    placeholder="https://prometheus.example.com/api/v1/write"
                    disabled={saving}
                  />
                </Field>
                <Field label={`${t("prometheus.username")} ${t("prometheus.optional")}`} htmlFor="st-prom-user">
                  <Input id="st-prom-user" value={draft[KEYS.remoteWriteUsername] ?? ""} onChange={(e) => set(KEYS.remoteWriteUsername, e.target.value)} placeholder="prometheus" disabled={saving} autoComplete="off" />
                </Field>
                <Field
                  label={`${t("prometheus.password")} ${t("prometheus.optional")}`}
                  htmlFor="st-prom-pass"
                  hint={passwordConfigured && !(draft[KEYS.remoteWritePassword] ?? "") ? t("prometheus.passwordConfigured") : undefined}
                >
                  <Input
                    id="st-prom-pass"
                    type="password"
                    value={draft[KEYS.remoteWritePassword] ?? ""}
                    onChange={(e) => set(KEYS.remoteWritePassword, e.target.value)}
                    placeholder={t("prometheus.passwordPlaceholder")}
                    disabled={saving}
                    autoComplete="new-password"
                  />
                </Field>
                <Field label={t("prometheus.pushInterval")} htmlFor="st-prom-interval">
                  <Select id="st-prom-interval" className="w-full" value={draft[KEYS.remoteWriteInterval] ?? "30s"} onChange={(e) => set(KEYS.remoteWriteInterval, e.target.value)} disabled={saving}>
                    {REMOTE_WRITE_INTERVALS.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
                  </Select>
                </Field>
                <div className="st-acts st-field st-end">
                  <Button
                    size="sm"
                    loading={testState === "testing"}
                    disabled={saving || !(draft[KEYS.remoteWriteURL] ?? "").trim()}
                    onClick={() => void testConnection()}
                  >
                    {t("prometheus.testConnection")}
                  </Button>
                  {testState === "ok" || testState === "error" ? (
                    <span className={`st-msg${testState === "error" ? " bad" : ""}`} role="status">{testMessage}</span>
                  ) : null}
                </div>
              </div>
            ) : null}

            <Row label={t("prometheus.processMetrics")} className="st-sw-row">
              <Switch checked={processMetricsEnabled} disabled={saving} onChange={(v) => set(KEYS.processMetricsEnabled, String(v))} label={t("prometheus.enableProcessMetrics")} />
            </Row>
            {processMetricsEnabled ? (
              <Row label={<label htmlFor="st-prom-topn">{t("prometheus.topNProcesses")}</label>}>
                <Select id="st-prom-topn" className="st-in-xs" value={draft[KEYS.processMetricsTopN] ?? "10"} onChange={(e) => set(KEYS.processMetricsTopN, e.target.value)} disabled={saving}>
                  {PROCESS_TOP_N_OPTIONS.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
                </Select>
              </Row>
            ) : null}
          </>
        )}

        <SheetFoot message={error ?? message} tone={error ? "bad" : undefined}>
          <Button size="sm" loading={saving} disabled={loading || !dirty} onClick={() => void save()}>
            {t("prometheus.saveSettings")}
          </Button>
        </SheetFoot>
      </Sheet>
    </Section>
  );
}
