"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef, useState, type FormEvent } from "react";
import type { QuickConnectParams } from "../../hooks/useSession";
import { Button } from "../ui/Button";
import { Select, Textarea } from "../ui/Input";
import { Modal } from "../ui/Modal";
import { formatAddress, parseAddress } from "./terminalTargets";

type AuthMethod = "password" | "private_key";
type Errors = { address?: string; secret?: string; key?: string };

interface QuickConnectDialogProps {
  open: boolean;
  onClose: () => void;
  onConnect: (params: QuickConnectParams) => void;
  /** The address to start from ("root@10.20.0.50:22"), from "Connect to <address>". */
  initialAddress?: string;
  /** What the person typed before stepping aside for the host key check; the form comes back with it. */
  draft?: QuickConnectParams | null;
}

/**
 * Connect to an address: SSH to a host that is not a device here (user@host:port) with a password or a private key,
 * the host key checked against the hub's known hosts unless turned off. Secrets are dropped when the form closes.
 */
export default function QuickConnectDialog({ open, onClose, onConnect, initialAddress = "", draft = null }: QuickConnectDialogProps) {
  const t = useTranslations("terminal.address");
  const [address, setAddress] = useState("");
  const [authMethod, setAuthMethod] = useState<AuthMethod>("password");
  const [password, setPassword] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [checkHostKey, setCheckHostKey] = useState(true);
  const [errors, setErrors] = useState<Errors>({});
  const addressRef = useRef<HTMLInputElement | null>(null);
  const secretRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!open) return undefined;
    setAddress(draft ? formatAddress({ user: draft.username, host: draft.host, port: draft.port ?? 22 }) : initialAddress);
    setAuthMethod(draft?.auth_method ?? "password");
    setPassword(draft?.password ?? "");
    setPrivateKey(draft?.private_key ?? "");
    setPassphrase(draft?.passphrase ?? "");
    setCheckHostKey(draft ? draft.strict_host_key !== false : true);
    setErrors({});
    const timer = window.setTimeout(() => (draft || initialAddress ? secretRef.current : addressRef.current)?.focus(), 30);
    return () => window.clearTimeout(timer);
  }, [open, initialAddress, draft]);

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const parsed = parseAddress(address);
    const next: Errors = {};
    if (!parsed) next.address = t("errAddress");
    else if (parsed.port < 1 || parsed.port > 65535) next.address = t("errPort");
    else if (!parsed.user) next.address = t("errUser", { host: parsed.host });
    if (authMethod === "password" && !password) next.secret = t("errPassword");
    if (authMethod === "private_key" && !privateKey.trim()) next.key = t("errKey");
    setErrors(next);
    if (!parsed || Object.keys(next).length > 0) {
      (next.address ? addressRef.current : secretRef.current)?.focus();
      return;
    }
    const params: QuickConnectParams = {
      host: parsed.host,
      port: parsed.port,
      username: parsed.user,
      auth_method: authMethod,
      strict_host_key: checkHostKey,
    };
    if (authMethod === "password") params.password = password;
    else {
      params.private_key = privateKey;
      if (passphrase) params.passphrase = passphrase;
    }
    setPassword("");
    setPrivateKey("");
    setPassphrase("");
    onConnect(params);
  };

  const hint = (id: string, text?: string) => (text ? <p className="kt-hint bad" id={id}>{text}</p> : null);

  return (
    <Modal open={open} onClose={onClose} title={t("title")}>
      <form className="kt-dlg-body tm-form" onSubmit={handleSubmit} noValidate>
        <div className="kt-field tm-wide">
          <label htmlFor="tm-qc-addr">{t("address")}</label>
          <input
            ref={addressRef}
            id="tm-qc-addr"
            className="kt-input tm-mono"
            value={address}
            placeholder="root@10.20.0.50:22"
            autoComplete="off"
            spellCheck={false}
            aria-invalid={errors.address ? true : undefined}
            aria-describedby={errors.address ? "tm-qc-addr-err" : undefined}
            onChange={(event) => setAddress(event.target.value)}
          />
          {hint("tm-qc-addr-err", errors.address)}
        </div>
        <div className="kt-field">
          <label htmlFor="tm-qc-auth">{t("login")}</label>
          <Select id="tm-qc-auth" value={authMethod} onChange={(event) => setAuthMethod(event.target.value as AuthMethod)}>
            <option value="password">{t("password")}</option>
            <option value="private_key">{t("privateKey")}</option>
          </Select>
        </div>
        {authMethod === "password" ? (
          <div className="kt-field">
            <label htmlFor="tm-qc-pass">{t("password")}</label>
            <input
              ref={secretRef}
              id="tm-qc-pass"
              className="kt-input"
              type="password"
              value={password}
              autoComplete="off"
              aria-invalid={errors.secret ? true : undefined}
              aria-describedby={errors.secret ? "tm-qc-pass-err" : undefined}
              onChange={(event) => setPassword(event.target.value)}
            />
            {hint("tm-qc-pass-err", errors.secret)}
          </div>
        ) : (
          <>
            <div className="kt-field tm-wide">
              <label htmlFor="tm-qc-key">{t("privateKey")}</label>
              <Textarea
                id="tm-qc-key"
                className="tm-mono"
                rows={4}
                spellCheck={false}
                value={privateKey}
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                error={Boolean(errors.key)}
                aria-describedby={errors.key ? "tm-qc-key-err" : undefined}
                onChange={(event) => setPrivateKey(event.target.value)}
              />
              {hint("tm-qc-key-err", errors.key)}
            </div>
            <div className="kt-field">
              <label htmlFor="tm-qc-phrase">{t("passphrase")}</label>
              <input
                ref={secretRef}
                id="tm-qc-phrase"
                className="kt-input"
                type="password"
                value={passphrase}
                placeholder={t("optional")}
                autoComplete="off"
                onChange={(event) => setPassphrase(event.target.value)}
              />
            </div>
          </>
        )}
        <div className="tm-wide tm-check">
          <label className="tm-cb">
            <input type="checkbox" checked={checkHostKey} onChange={(event) => setCheckHostKey(event.target.checked)} />
            {t("checkHostKey")}
          </label>
          {checkHostKey ? null : <p className="kt-hint">{t("uncheckedHint")}</p>}
        </div>
        <div className="kt-dlg-acts tm-wide">
          <Button type="button" variant="ghost" onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" variant="primary">{t("connect")}</Button>
        </div>
      </form>
    </Modal>
  );
}
