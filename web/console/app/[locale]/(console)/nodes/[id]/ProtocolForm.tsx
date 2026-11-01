"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { CredentialProfileSelect } from "../../../../components/credentials/CredentialProfileSelect";
import { useCredentialProfiles } from "../../../../hooks/useCredentialProfiles";
import type { ProtocolConfig, ProtocolType, TestResult } from "./useProtocolConfigs";
import {
  KeyInstallPrompt,
  OptionCheck,
  RdpOptions,
  SshOptions,
  TestResultLine,
  TextField,
  type RdpOptionsState,
} from "./settingsViewProtocolOptions";

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

const PROTOCOL_CREDENTIAL_KINDS: Record<ProtocolType, readonly string[]> = {
  ssh: ["ssh_password", "ssh_private_key", "hub_ssh_identity"],
  telnet: ["telnet_password"],
  vnc: ["vnc_password"],
  rdp: ["rdp_password"],
  ard: ["vnc_password"],
};

type ProtocolFormProps = {
  assetId: string;
  initial?: Partial<ProtocolConfig>;
  editMode?: boolean;
  onSave: (data: Partial<ProtocolConfig>) => Promise<{ ok: boolean; error?: string }>;
  onTest: (protocol: ProtocolType) => Promise<TestResult>;
  onPushHubKey?: () => Promise<{ ok: boolean; error?: string }>;
  onCancel: () => void;
};

const configBool = (initial: Partial<ProtocolConfig> | undefined, key: string) =>
  (initial?.config?.[key] as boolean | undefined) ?? false;
const configText = (initial: Partial<ProtocolConfig> | undefined, key: string) =>
  (initial?.config?.[key] as string | undefined) ?? "";

/** Add or edit one remote protocol: where it connects, which login it uses, its options, Test and Save. */
export function ProtocolForm({
  assetId: _assetId,
  initial,
  editMode = false,
  onSave,
  onTest,
  onPushHubKey,
  onCancel,
}: ProtocolFormProps) {
  const t = useTranslations("devices.settingsTab.protocolForm");
  const [protocol, setProtocol] = useState<ProtocolType>(initial?.protocol ?? "ssh");
  const [host, setHost] = useState(initial?.host ?? "");
  const [port, setPort] = useState<number>(initial?.port ?? DEFAULT_PORTS[initial?.protocol ?? "ssh"]);
  const [username, setUsername] = useState(initial?.username ?? "");
  const [credentialProfileId, setCredentialProfileId] = useState(initial?.credential_profile_id ?? "");
  const credentialProfiles = useCredentialProfiles();

  const [strictHostKey, setStrictHostKey] = useState<boolean>(configBool(initial, "strict_host_key"));
  const [hostKey, setHostKey] = useState<string>(configText(initial, "host_key"));
  const [allowInsecureVNC, setAllowInsecureVNC] = useState<boolean>(configBool(initial, "allow_insecure_vnc"));
  const [allowInsecureTelnet, setAllowInsecureTelnet] = useState<boolean>(configBool(initial, "allow_insecure_telnet"));
  const [rdp, setRdp] = useState<RdpOptionsState>({
    domain: configText(initial, "domain"),
    nla: configBool(initial, "nla_enabled"),
    ignoreCertificate: configBool(initial, "ignore_certificate"),
    allowLegacySecurity: configBool(initial, "allow_legacy_security"),
    fingerprints: configText(initial, "certificate_fingerprints"),
  });

  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [pushingKey, setPushingKey] = useState(false);
  const [formError, setFormError] = useState("");
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [showKeyInstallBanner, setShowKeyInstallBanner] = useState(false);

  const handleProtocolChange = (next: ProtocolType) => {
    setProtocol(next);
    setPort(DEFAULT_PORTS[next]);
    setCredentialProfileId("");
    setHostKey("");
    setAllowInsecureVNC(false);
    setAllowInsecureTelnet(false);
    setTestResult(null);
    setShowKeyInstallBanner(false);
  };

  const buildConfig = (): Record<string, unknown> => {
    const cfg: Record<string, unknown> = {};
    if (protocol === "ssh") {
      cfg["strict_host_key"] = strictHostKey;
      if (hostKey.trim()) cfg["host_key"] = hostKey.trim();
    }
    if (protocol === "rdp") {
      if (rdp.domain.trim()) cfg["domain"] = rdp.domain.trim();
      cfg["nla_enabled"] = rdp.nla;
      cfg["ignore_certificate"] = rdp.ignoreCertificate;
      cfg["allow_legacy_security"] = rdp.allowLegacySecurity;
      if (rdp.fingerprints.trim()) cfg["certificate_fingerprints"] = rdp.fingerprints.trim();
    }
    if (protocol === "vnc" || protocol === "ard") cfg["allow_insecure_vnc"] = allowInsecureVNC;
    if (protocol === "telnet") cfg["allow_insecure_telnet"] = allowInsecureTelnet;
    return cfg;
  };

  const handleSave = async () => {
    setFormError("");
    setSaving(true);
    try {
      const data: Partial<ProtocolConfig> = { protocol, port, config: buildConfig() };
      if (host.trim()) data.host = host.trim();
      if (username.trim()) data.username = username.trim();
      data.credential_profile_id = credentialProfileId.trim();
      const result = await onSave(data);
      if (!result.ok) setFormError(result.error ?? t("saveFailed"));
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
      if (result.success && protocol === "ssh") setShowKeyInstallBanner(true);
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
      if (!result.ok) setFormError(result.error ?? t("pushFailed"));
      else setShowKeyInstallBanner(false);
    } finally {
      setPushingKey(false);
    }
  };

  const insecureBlocked = ((protocol === "vnc" || protocol === "ard") && !allowInsecureVNC) || (protocol === "telnet" && !allowInsecureTelnet);

  return (
    <div className="dv-form">
      {formError ? <p className="dv-err" role="alert">{formError}</p> : null}

      <div className="dv-form-row">
        <div className="dv-field">
          <label htmlFor="protocol-kind">{t("protocol")}</label>
          <select
            id="protocol-kind"
            className="dv-input"
            value={protocol}
            disabled={editMode}
            onChange={(e) => handleProtocolChange(e.target.value as ProtocolType)}
          >
            {PROTOCOL_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>
        <TextField id="protocol-host" label={t("host")} value={host} onChange={setHost} placeholder={t("hostPlaceholder")} />
        <TextField
          id="protocol-port"
          label={t("port")}
          type="number"
          min={1}
          max={65535}
          value={port}
          onChange={(value) => setPort(Number(value) || DEFAULT_PORTS[protocol])}
        />
      </div>

      {protocol === "telnet" && (
        <OptionCheck warn checked={allowInsecureTelnet} onChange={setAllowInsecureTelnet} label={t("telnetOptIn")} hint={t("telnetOptInHint")} />
      )}
      {(protocol === "vnc" || protocol === "ard") && (
        <OptionCheck warn checked={allowInsecureVNC} onChange={setAllowInsecureVNC} label={t("vncOptIn")} hint={t("vncOptInHint")} />
      )}

      <div className="dv-form-row">
        {PROTOCOLS_WITH_USERNAME.includes(protocol) && (
          <TextField
            id="protocol-username"
            label={t("username")}
            value={username}
            onChange={setUsername}
            placeholder={protocol === "rdp" ? "DOMAIN\\user" : "root"}
          />
        )}
        <div className="dv-field dst-cred">
          <CredentialProfileSelect
            id="protocol-credential-profile"
            label={t("credential")}
            value={credentialProfileId}
            onChange={setCredentialProfileId}
            profiles={credentialProfiles.profiles}
            loading={credentialProfiles.loading}
            error={credentialProfiles.error}
            allowedKinds={PROTOCOL_CREDENTIAL_KINDS[protocol]}
          />
          <p className="dv-hint">{t("credentialHint")}</p>
        </div>
      </div>

      {protocol === "ssh" && (
        <SshOptions
          strictHostKey={strictHostKey}
          onStrictHostKeyChange={setStrictHostKey}
          hostKey={hostKey}
          onHostKeyChange={setHostKey}
        />
      )}
      {protocol === "rdp" && <RdpOptions value={rdp} onChange={setRdp} />}

      {testResult !== null && <TestResultLine protocol={protocol} result={testResult} />}

      {showKeyInstallBanner && protocol === "ssh" && onPushHubKey && (
        <KeyInstallPrompt busy={pushingKey} onInstall={() => void handlePushKey()} onSkip={() => setShowKeyInstallBanner(false)} />
      )}

      <div className="dv-form-acts">
        <Button size="sm" variant="ghost" onClick={onCancel}>{t("cancel")}</Button>
        <Button size="sm" disabled={testing || saving} onClick={() => void handleTest()}>
          {testing ? t("testing") : protocol === "rdp" ? t("testRdp") : t("test")}
        </Button>
        <Button size="sm" variant="primary" disabled={saving || testing || insecureBlocked} onClick={() => void handleSave()}>
          {saving ? t("saving") : t("save")}
        </Button>
      </div>
    </div>
  );
}
