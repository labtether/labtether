"use client";

import { useTranslations } from "next-intl";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Modal";

/** What the Hub found for a host's SSH key (POST /terminal/host-keys/check). */
export type HostKeyCheck = {
  status?: "trusted" | "unknown" | "changed" | "unchecked";
  host?: string;
  port?: number;
  host_key?: string;
  fingerprint?: string;
  key_type?: string;
  saved_fingerprint?: string;
};

export type HostKeyDialogPhase = "checking" | "unknown" | "changed" | "error";

type Props = {
  phase: HostKeyDialogPhase;
  target: { host: string; port: number };
  check?: HostKeyCheck;
  error?: string;
  busy: boolean;
  onTrust: () => void;
  onRemove: () => void;
  onCancel: () => void;
};

/** The host key file on a typical Linux host, by key type, for "ssh-keygen -lf". */
function keyFile(keyType = ""): { label: string; file: string } {
  if (keyType.startsWith("ecdsa-")) return { label: "ECDSA", file: "/etc/ssh/ssh_host_ecdsa_key.pub" };
  if (keyType === "ssh-rsa") return { label: "RSA", file: "/etc/ssh/ssh_host_rsa_key.pub" };
  return { label: "ED25519", file: "/etc/ssh/ssh_host_ed25519_key.pub" };
}

/** Trust on first use: the fingerprint with Trust and connect, or a changed key with Remove saved key. */
export default function HostKeyDialog({ phase, target, check, error, busy, onTrust, onRemove, onCancel }: Props) {
  const t = useTranslations("terminal.hostKey");
  const where = `${check?.host ?? target.host}:${check?.port ?? target.port}`;
  const key = keyFile(check?.key_type);
  const title = phase === "changed" ? t("titleChanged") : t("titleUnknown");
  return (
    <Modal open onClose={onCancel} title={title}>
      <div className="kt-dlg-body tm-form">
        {phase === "checking" ? <p className="tm-wide">{t("checking")}</p> : null}
        {phase === "unknown" ? (
          <>
            <p className="tm-wide">{t("unknownBody", { target: where })}</p>
            <div className="kt-field tm-wide">
              <span className="kt-label">{t("fingerprint", { type: key.label })}</span>
              <code className="tm-mono tm-hk-fp">{check?.fingerprint}</code>
            </div>
            <p className="tm-wide">{t.rich("compare", { command: () => <code className="tm-mono">ssh-keygen -lf {key.file}</code> })}</p>
          </>
        ) : null}
        {phase === "changed" ? (
          <>
            <p className="tm-wide">{t("changedBody", { target: where })}</p>
            <div className="kt-field tm-wide">
              <span className="kt-label">{t("saved")}</span>
              <code className="tm-mono tm-hk-fp">{check?.saved_fingerprint}</code>
            </div>
            <div className="kt-field tm-wide">
              <span className="kt-label">{t("now", { type: key.label })}</span>
              <code className="tm-mono tm-hk-fp">{check?.fingerprint}</code>
            </div>
          </>
        ) : null}
        {phase === "error" || error ? <p className="kt-hint bad tm-wide" role="alert">{error || t("failed", { target: where })}</p> : null}
        <div className="kt-dlg-acts tm-wide">
          <Button type="button" variant="ghost" onClick={onCancel}>{t("cancel")}</Button>
          {phase === "unknown" ? <Button type="button" variant="primary" loading={busy} onClick={onTrust}>{t("trust")}</Button> : null}
          {phase === "changed" ? <Button type="button" variant="danger" loading={busy} onClick={onRemove}>{t("remove")}</Button> : null}
        </div>
      </div>
    </Modal>
  );
}
