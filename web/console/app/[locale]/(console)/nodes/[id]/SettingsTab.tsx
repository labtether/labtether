"use client";

import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { Badge } from "../../../../components/ui/Badge";
import { useCollectorSettings } from "../../../../hooks/useCollectorSettings";

export function SettingsTab({ collectorId, loading }: { collectorId: string | null; loading: boolean }) {
  const s = useCollectorSettings(collectorId);

  return (
    <Card className="mb-4">
      <h2 className="text-sm font-medium text-[var(--text)] mb-3">Collector Settings</h2>
      {(loading || s.loading) ? (
        <p className="text-sm text-[var(--muted)]">Loading collector settings...</p>
      ) : !collectorId ? (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No collector found</p>
          <p className="text-xs text-[var(--muted)] text-center max-w-sm">This asset does not have an associated hub collector. Configure it from the global Settings page first.</p>
        </div>
      ) : (
        <>
          {s.error ? <p className="text-xs text-[var(--bad)] mb-3">{s.error}</p> : null}
          <div className="divide-y divide-[var(--line)]">
            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Status</span>
                <span className="text-xs text-[var(--muted)]">Collector configuration state</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Badge status={s.configured ? "ok" : "warning"} size="sm" />
                {s.credentialName ? <span className="text-xs text-[var(--muted)]">Credential: {s.credentialName}</span> : null}
              </div>
            </div>

            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Base URL</span>
                <span className="text-xs text-[var(--muted)]">Example: https://pve.local:8006</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Input value={s.baseURL} onChange={(e) => s.setBaseURL(e.target.value)} placeholder="https://pve.local:8006" />
              </div>
            </div>

            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Authentication</span>
                <span className="text-xs text-[var(--muted)]">API Token or Username & Password</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {(["api_token", "password"] as const).map((m) => (
                  <button
                    key={m}
                    type="button"
                    onClick={() => s.setAuthMethod(m)}
                    className={`px-3 py-1.5 text-xs font-medium rounded-md border transition-colors duration-150 ${
                      s.authMethod === m
                        ? "border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]"
                        : "border-[var(--line)] bg-transparent text-[var(--muted)] hover:border-[var(--muted)]"
                    }`}
                  >
                    {m === "api_token" ? "API Token" : "Username & Password"}
                  </button>
                ))}
              </div>
            </div>

            {s.authMethod === "api_token" ? (
              <>
                <div className="py-3">
                  <div className="rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-3 py-2">
                    <p className="text-xs text-[var(--warn)]">
                      API tokens do not support Proxmox console/terminal access due to a known Proxmox limitation (bug #6079). Terminal sessions will fall back to SSH.
                    </p>
                  </div>
                </div>
                <div className="flex items-start justify-between gap-4 py-3">
                  <div className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium text-[var(--text)]">Token ID</span>
                    <span className="text-xs text-[var(--muted)]">Example: labtether@pve!monitoring</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Input value={s.tokenID} onChange={(e) => s.setTokenID(e.target.value)} placeholder="labtether@pve!monitoring" />
                  </div>
                </div>
                <div className="flex items-start justify-between gap-4 py-3">
                  <div className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium text-[var(--text)]">Token Secret</span>
                    <span className="text-xs text-[var(--muted)]">Leave blank to keep existing secret unchanged</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Input
                      type="password"
                      value={s.tokenSecret}
                      onChange={(e) => s.setTokenSecret(e.target.value)}
                      placeholder={s.configured ? "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022 (unchanged)" : "Required for initial setup"}
                    />
                  </div>
                </div>
              </>
            ) : (
              <>
                <div className="flex items-start justify-between gap-4 py-3">
                  <div className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium text-[var(--text)]">Username</span>
                    <span className="text-xs text-[var(--muted)]">Example: root@pam</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Input value={s.username} onChange={(e) => s.setUsername(e.target.value)} placeholder="root@pam" />
                  </div>
                </div>
                <div className="flex items-start justify-between gap-4 py-3">
                  <div className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium text-[var(--text)]">Password</span>
                    <span className="text-xs text-[var(--muted)]">Leave blank to keep existing password unchanged</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Input
                      type="password"
                      value={s.tokenSecret}
                      onChange={(e) => s.setTokenSecret(e.target.value)}
                      placeholder={s.configured ? "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022 (unchanged)" : "Required for initial setup"}
                    />
                  </div>
                </div>
              </>
            )}

            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Cluster Name</span>
                <span className="text-xs text-[var(--muted)]">Optional display label</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Input value={s.clusterName} onChange={(e) => s.setClusterName(e.target.value)} placeholder="Homelab Proxmox" />
              </div>
            </div>

            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Poll Interval (seconds)</span>
                <span className="text-xs text-[var(--muted)]">Collector refresh cadence</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Input type="number" min={15} max={3600} value={s.intervalSeconds} onChange={(e) => s.setIntervalSeconds(Number(e.target.value) || 60)} />
              </div>
            </div>

            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">TLS</span>
                <span className="text-xs text-[var(--muted)]">Self-signed certs are common in homelabs</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <label className="text-xs text-[var(--muted)]">
                  <input type="checkbox" checked={s.skipVerify} onChange={(e) => s.setSkipVerify(e.target.checked)} />{" "}
                  Skip certificate verification
                </label>
              </div>
            </div>

            <div className="flex items-start justify-between gap-4 py-3">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Custom CA PEM</span>
                <span className="text-xs text-[var(--muted)]">Optional PEM certificate for TLS verification</span>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <textarea
                  className="w-full bg-transparent border border-[var(--line)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:border-[var(--muted)] transition-colors duration-150 resize-y"
                  rows={4}
                  value={s.caPEM}
                  onChange={(e) => s.setCAPEM(e.target.value)}
                  placeholder="-----BEGIN CERTIFICATE-----"
                />
              </div>
            </div>
          </div>

          <div className="flex items-center gap-3 pt-4">
            <Button disabled={s.testing || s.saving} onClick={() => void s.testConnection()}>
              {s.testing ? "Testing..." : "Test Connection"}
            </Button>
            <Button disabled={s.running || s.testing || s.saving} onClick={() => void s.runNow()}>
              {s.running ? "Starting..." : "Run Now"}
            </Button>
            <Button variant="primary" disabled={s.saving || s.testing} onClick={() => void s.save()}>
              {s.saving ? "Saving..." : "Save Settings"}
            </Button>
            {s.message ? <span className="text-xs text-[var(--muted)]">{s.message}</span> : null}
          </div>
        </>
      )}
    </Card>
  );
}
