"use client";

import { useState } from "react";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import type { ProtocolConfig, ProtocolType, TestResult } from "./useProtocolConfigs";

const DEFAULT_PORTS: Record<ProtocolType, number> = {
  ssh: 22,
  telnet: 23,
  vnc: 5900,
  rdp: 3389,
  ard: 5900,
};

const PROTOCOL_OPTIONS: { value: ProtocolType; label: string }[] = [
  { value: "ssh", label: "SSH" },
  { value: "telnet", label: "Telnet" },
  { value: "vnc", label: "VNC" },
  { value: "rdp", label: "RDP" },
  { value: "ard", label: "ARD (Apple Remote Desktop)" },
];

const PROTOCOLS_WITH_USERNAME: ProtocolType[] = ["ssh", "telnet", "rdp"];

type ProtocolFormProps = {
  assetId: string;
  initial?: Partial<ProtocolConfig>;
  editMode?: boolean;
  onSave: (data: Partial<ProtocolConfig>) => Promise<{ ok: boolean; error?: string }>;
  onTest: (protocol: ProtocolType) => Promise<TestResult>;
  onPushHubKey?: () => Promise<{ ok: boolean; error?: string }>;
  onCancel: () => void;
};

export function ProtocolForm({
  assetId: _assetId,
  initial,
  editMode = false,
  onSave,
  onTest,
  onPushHubKey,
  onCancel,
}: ProtocolFormProps) {
  const [protocol, setProtocol] = useState<ProtocolType>(initial?.protocol ?? "ssh");
  const [host, setHost] = useState(initial?.host ?? "");
  const [port, setPort] = useState<number>(initial?.port ?? DEFAULT_PORTS[initial?.protocol ?? "ssh"]);
  const [username, setUsername] = useState(initial?.username ?? "");
  const [credentialProfileId, setCredentialProfileId] = useState(initial?.credential_profile_id ?? "");

  // SSH-specific
  const [strictHostKey, setStrictHostKey] = useState<boolean>((initial?.config?.["strict_host_key"] as boolean | undefined) ?? false);

  // RDP-specific
  const [rdpDomain, setRdpDomain] = useState<string>((initial?.config?.["domain"] as string | undefined) ?? "");
  const [nla, setNla] = useState<boolean>((initial?.config?.["nla_enabled"] as boolean | undefined) ?? false);

  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [pushingKey, setPushingKey] = useState(false);
  const [formError, setFormError] = useState("");
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [showKeyInstallBanner, setShowKeyInstallBanner] = useState(false);

  const handleProtocolChange = (next: ProtocolType) => {
    setProtocol(next);
    setPort(DEFAULT_PORTS[next]);
    setTestResult(null);
    setShowKeyInstallBanner(false);
  };

  const buildConfig = (): Record<string, unknown> => {
    const cfg: Record<string, unknown> = {};
    if (protocol === "ssh") {
      cfg["strict_host_key"] = strictHostKey;
    }
    if (protocol === "rdp") {
      if (rdpDomain.trim()) cfg["domain"] = rdpDomain.trim();
      cfg["nla_enabled"] = nla;
    }
    return cfg;
  };

  const handleSave = async () => {
    setFormError("");
    setSaving(true);
    try {
      const data: Partial<ProtocolConfig> = {
        protocol,
        port,
        config: buildConfig(),
      };
      if (host.trim()) data.host = host.trim();
      if (username.trim()) data.username = username.trim();
      if (credentialProfileId.trim()) data.credential_profile_id = credentialProfileId.trim();

      const result = await onSave(data);
      if (!result.ok) {
        setFormError(result.error ?? "Failed to save.");
      }
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    setFormError("");
    setTestResult(null);
    setShowKeyInstallBanner(false);
    setTesting(true);
    try {
      const result = await onTest(protocol);
      setTestResult(result);
      if (result.success && protocol === "ssh") {
        setShowKeyInstallBanner(true);
      }
    } finally {
      setTesting(false);
    }
  };

  const handlePushKey = async () => {
    if (!onPushHubKey) return;
    setPushingKey(true);
    setFormError("");
    try {
      const result = await onPushHubKey();
      if (!result.ok) {
        setFormError(result.error ?? "Failed to push hub key.");
      } else {
        setShowKeyInstallBanner(false);
      }
    } finally {
      setPushingKey(false);
    }
  };

  const showUsername = PROTOCOLS_WITH_USERNAME.includes(protocol);

  return (
    <div className="space-y-3">
      {formError ? <p className="text-xs text-[var(--bad)]">{formError}</p> : null}

      {/* Protocol selector — disabled in edit mode */}
      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Protocol</label>
        <select
          value={protocol}
          disabled={editMode}
          onChange={(e) => handleProtocolChange(e.target.value as ProtocolType)}
          className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-2 text-xs text-[var(--text)] disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {PROTOCOL_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      {/* Telnet warning */}
      {protocol === "telnet" && (
        <div className="rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-3 py-2">
          <p className="text-xs text-[var(--warn)]">
            Telnet connections are unencrypted. Credentials and data are sent in plaintext.
          </p>
        </div>
      )}

      {/* Host override */}
      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Host override</label>
        <Input
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="Inherits from device if empty"
        />
      </div>

      {/* Port */}
      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Port</label>
        <Input
          type="number"
          min={1}
          max={65535}
          value={port}
          onChange={(e) => setPort(Number(e.target.value) || DEFAULT_PORTS[protocol])}
        />
      </div>

      {/* Username */}
      {showUsername && (
        <div>
          <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Username</label>
          <Input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder={protocol === "rdp" ? "DOMAIN\\user or user" : "admin"}
          />
        </div>
      )}

      {/* Credential profile */}
      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Credential Profile ID</label>
        <Input
          value={credentialProfileId}
          onChange={(e) => setCredentialProfileId(e.target.value)}
          placeholder="Optional credential profile ID"
        />
        <p className="mt-1 text-[11px] text-[var(--muted)]">Leave blank to use the default credential for this device.</p>
      </div>

      {/* SSH-specific */}
      {protocol === "ssh" && (
        <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
          <input
            type="checkbox"
            checked={strictHostKey}
            onChange={(e) => setStrictHostKey(e.target.checked)}
          />
          Strict host key checking
        </label>
      )}

      {/* RDP-specific */}
      {protocol === "rdp" && (
        <>
          <div>
            <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Domain</label>
            <Input
              value={rdpDomain}
              onChange={(e) => setRdpDomain(e.target.value)}
              placeholder="WORKGROUP or corp.example.com"
            />
          </div>
          <label className="flex items-center gap-2 text-xs text-[var(--muted)]">
            <input
              type="checkbox"
              checked={nla}
              onChange={(e) => setNla(e.target.checked)}
            />
            Network Level Authentication (NLA)
          </label>
        </>
      )}

      {/* Test result */}
      {testResult !== null && (
        <div
          className={`rounded-lg border px-3 py-2 ${
            testResult.success
              ? "border-[var(--ok)]/30 bg-[var(--ok-glow)]"
              : "border-[var(--bad)]/30 bg-[var(--bad-glow)]"
          }`}
        >
          {testResult.success ? (
            <p className="text-xs text-[var(--ok)]">
              Connection successful{testResult.latency_ms > 0 ? ` (${testResult.latency_ms}ms)` : ""}.
            </p>
          ) : (
            <p className="text-xs text-[var(--bad)]">
              {testResult.error ?? "Connection failed."}
            </p>
          )}
        </div>
      )}

      {/* SSH key install banner */}
      {showKeyInstallBanner && protocol === "ssh" && onPushHubKey && (
        <div className="rounded-lg border border-[var(--ok)]/30 bg-[var(--ok-glow)] px-3 py-2">
          <p className="text-xs text-[var(--ok)] mb-2">
            Connection works! Install hub SSH key for passwordless access?
          </p>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="primary"
              disabled={pushingKey}
              onClick={() => void handlePushKey()}
            >
              {pushingKey ? "Installing..." : "Install"}
            </Button>
            <Button
              size="sm"
              onClick={() => setShowKeyInstallBanner(false)}
            >
              Skip
            </Button>
          </div>
        </div>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onCancel}>Cancel</Button>
        <Button
          disabled={testing || saving}
          onClick={() => void handleTest()}
        >
          {testing ? "Testing..." : "Test Connection"}
        </Button>
        <Button
          variant="primary"
          disabled={saving || testing}
          onClick={() => void handleSave()}
        >
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}
