"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Button } from "../../../../components/ui/Button";
import type { ProtocolType, TestResult } from "./useProtocolConfigs";

/** A checkbox with its label; warn: the choice weakens security (label in the warning tone, the reason under it). */
export function OptionCheck({
  checked,
  onChange,
  label,
  hint,
  warn = false,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  hint?: string;
  warn?: boolean;
}) {
  return (
    <label className={`dst-check dst-check-block${warn ? " warn" : ""}`}>
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span>
        <span className="dst-check-l">{label}</span>
        {hint ? <span className="dv-hint dst-block">{hint}</span> : null}
      </span>
    </label>
  );
}

/** A labelled text field of the protocol form. */
export function TextField({
  id,
  label,
  value,
  onChange,
  placeholder,
  hint,
  mono = false,
  type = "text",
  min,
  max,
}: {
  id: string;
  label: string;
  value: string | number;
  onChange: (value: string) => void;
  placeholder?: string;
  hint?: ReactNode;
  mono?: boolean;
  type?: "text" | "number";
  min?: number;
  max?: number;
}) {
  return (
    <div className="dv-field">
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        className={`dv-input${mono ? " dv-mono" : ""}`}
        type={type}
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        spellCheck={false}
        autoComplete="off"
      />
      {hint ? <p className="dv-hint">{hint}</p> : null}
    </div>
  );
}

/** SSH: host key checking and the expected host key. */
export function SshOptions({
  strictHostKey,
  onStrictHostKeyChange,
  hostKey,
  onHostKeyChange,
}: {
  strictHostKey: boolean;
  onStrictHostKeyChange: (value: boolean) => void;
  hostKey: string;
  onHostKeyChange: (value: string) => void;
}) {
  const t = useTranslations("devices.settingsTab.protocolForm");
  return (
    <>
      <OptionCheck checked={strictHostKey} onChange={onStrictHostKeyChange} label={t("strictHostKey")} />
      <TextField
        id="protocol-host-key"
        label={t("hostKey")}
        value={hostKey}
        onChange={onHostKeyChange}
        placeholder="ssh-ed25519 AAAA…"
        hint={t("hostKeyHint")}
        mono
      />
    </>
  );
}

export type RdpOptionsState = {
  domain: string;
  nla: boolean;
  fingerprints: string;
  ignoreCertificate: boolean;
  allowLegacySecurity: boolean;
};

/** RDP: domain, NLA, a pinned certificate, and the two insecure opt-ins (each switches off what it conflicts with). */
export function RdpOptions({ value, onChange }: { value: RdpOptionsState; onChange: (next: RdpOptionsState) => void }) {
  const t = useTranslations("devices.settingsTab.protocolForm");
  const set = (patch: Partial<RdpOptionsState>) => onChange({ ...value, ...patch });
  return (
    <>
      <TextField id="protocol-rdp-domain" label={t("domain")} value={value.domain} onChange={(domain) => set({ domain })} placeholder="WORKGROUP" />
      <OptionCheck
        checked={value.nla}
        onChange={(nla) => set(nla ? { nla, allowLegacySecurity: false } : { nla })}
        label={t("nla")}
      />
      <TextField
        id="protocol-rdp-fingerprint"
        label={t("fingerprint")}
        value={value.fingerprints}
        onChange={(fingerprints) => set(fingerprints.trim() ? { fingerprints, ignoreCertificate: false, allowLegacySecurity: false } : { fingerprints })}
        placeholder="sha256:AA:BB:…"
        hint={t("fingerprintHint")}
        mono
      />
      <OptionCheck
        warn
        checked={value.ignoreCertificate}
        onChange={(ignoreCertificate) =>
          set(ignoreCertificate ? { ignoreCertificate, allowLegacySecurity: false, fingerprints: "" } : { ignoreCertificate })}
        label={t("ignoreCert")}
        hint={t("ignoreCertHint")}
      />
      <OptionCheck
        warn
        checked={value.allowLegacySecurity}
        onChange={(allowLegacySecurity) =>
          set(allowLegacySecurity
            ? { allowLegacySecurity, nla: false, ignoreCertificate: false, fingerprints: "" }
            : { allowLegacySecurity })}
        label={t("legacy")}
        hint={t("legacyHint")}
      />
    </>
  );
}

/** What the last Test found, as one line in its tone. */
export function TestResultLine({ protocol, result }: { protocol: ProtocolType; result: TestResult }) {
  const t = useTranslations("devices.settingsTab.protocolForm");
  if (!result.success) {
    return (
      <p className="dst-result bad" role="status">
        <span className="dv-dot bad" aria-hidden="true" />
        {result.error ?? t("testFailed")}
      </p>
    );
  }
  const ms = result.latency_ms > 0 ? result.latency_ms : 0;
  const text = protocol === "rdp"
    ? (ms ? t("rdpOk", { ms }) : t("rdpOkNoTime"))
    : (ms ? t("testOk", { ms }) : t("testOkNoTime"));
  return <p className="dst-result" role="status">{text}</p>;
}

/** After a working SSH test: offer to install the hub's key on the device. */
export function KeyInstallPrompt({ busy, onInstall, onSkip }: { busy: boolean; onInstall: () => void; onSkip: () => void }) {
  const t = useTranslations("devices.settingsTab.protocolForm");
  return (
    <div className="dst-prompt">
      <span>{t("keyPrompt")}</span>
      <span className="dv-sp dst-grow" />
      <Button size="sm" variant="ghost" onClick={onSkip}>{t("skip")}</Button>
      <Button size="sm" disabled={busy} onClick={onInstall}>{busy ? t("installing") : t("install")}</Button>
    </div>
  );
}
