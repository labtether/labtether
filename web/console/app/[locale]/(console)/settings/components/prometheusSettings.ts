export const PROMETHEUS_SETTING_KEYS = {
  scrapeEnabled: "prometheus.scrape_enabled",
  remoteWriteEnabled: "prometheus.remote_write_enabled",
  remoteWriteURL: "prometheus.remote_write_url",
  remoteWriteUsername: "prometheus.remote_write_username",
  remoteWritePassword: "prometheus.remote_write_password",
  remoteWriteInterval: "prometheus.remote_write_interval",
  processMetricsEnabled: "prometheus.process_metrics_enabled",
  processMetricsTopN: "prometheus.process_metrics_top_n",
} as const;

export type PrometheusSettingsMap = Record<string, string>;

export type PrometheusRuntimeSetting = {
  key: string;
  effective_value?: string;
  sensitive?: boolean;
  configured?: boolean;
};

export function buildPrometheusSettingsState(entries: PrometheusRuntimeSetting[]): {
  values: PrometheusSettingsMap;
  passwordConfigured: boolean;
} {
  const values: PrometheusSettingsMap = {};
  let passwordConfigured = false;
  for (const entry of entries) {
    if (!Object.values(PROMETHEUS_SETTING_KEYS).includes(entry.key as typeof PROMETHEUS_SETTING_KEYS[keyof typeof PROMETHEUS_SETTING_KEYS])) {
      continue;
    }
    if (entry.key === PROMETHEUS_SETTING_KEYS.remoteWritePassword) {
      // Treat the server response as untrusted and never hydrate a secret into
      // browser state, even if an older backend accidentally sends one.
      values[entry.key] = "";
      passwordConfigured = entry.configured === true;
      continue;
    }
    values[entry.key] = typeof entry.effective_value === "string" ? entry.effective_value : "";
  }
  return { values, passwordConfigured };
}

export function buildPrometheusPatch(
  draft: PrometheusSettingsMap,
  current: PrometheusSettingsMap,
): PrometheusSettingsMap {
  const values: PrometheusSettingsMap = {};
  for (const key of Object.values(PROMETHEUS_SETTING_KEYS)) {
	const rawNext = draft[key] ?? "";
	const next = key === PROMETHEUS_SETTING_KEYS.remoteWritePassword ? rawNext : rawNext.trim();
	if (key === PROMETHEUS_SETTING_KEYS.remoteWritePassword && next === "") {
      // An empty password field means unchanged. Clearing the stored override
      // remains an explicit Reset action in Advanced Settings.
      continue;
    }
    if (next !== (current[key] ?? "").trim()) {
      values[key] = next;
    }
  }
  return values;
}

export function buildPrometheusTestRequest(
  draft: PrometheusSettingsMap,
  passwordConfigured: boolean,
) {
  const password = draft[PROMETHEUS_SETTING_KEYS.remoteWritePassword] ?? "";
  return {
    url: (draft[PROMETHEUS_SETTING_KEYS.remoteWriteURL] ?? "").trim(),
    username: (draft[PROMETHEUS_SETTING_KEYS.remoteWriteUsername] ?? "").trim(),
    password,
    use_stored_password: password === "" && passwordConfigured,
  };
}
