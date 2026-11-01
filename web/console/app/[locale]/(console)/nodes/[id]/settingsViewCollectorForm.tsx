"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import type { useCollectorSettings } from "../../../../hooks/useCollectorSettings";
import { OptionCheck, TextField } from "./settingsViewProtocolOptions";

type CollectorSettings = ReturnType<typeof useCollectorSettings>;

/** The Proxmox API connection as a form: URL, token or user and password, cluster name, poll, TLS; Test and Save. */
export function CollectorForm({ s, onCancel }: { s: CollectorSettings; onCancel: () => void }) {
  const t = useTranslations("devices.settingsTab.collector");
  const token = s.authMethod === "api_token";
  const keepSaved = s.configured ? t("secretKeep") : t("secretRequired");
  return (
    <div className="dv-sheet dv-form dst-collector-form">
      <div className="dv-form-row">
        <TextField id="dst-col-url" label={t("url")} value={s.baseURL} onChange={s.setBaseURL} placeholder="https://10.20.0.11:8006" />
        <TextField id="dst-col-name" label={t("clusterName")} value={s.clusterName} onChange={s.setClusterName} placeholder={t("clusterNamePlaceholder")} />
      </div>
      <div className="dv-field">
        <span className="dv-lbl">{t("auth")}</span>
        <div className="dv-seg" role="group" aria-label={t("auth")}>
          <button type="button" aria-pressed={token} onClick={() => s.setAuthMethod("api_token")}>{t("authToken")}</button>
          <button type="button" aria-pressed={!token} onClick={() => s.setAuthMethod("password")}>{t("authPassword")}</button>
        </div>
        {token ? <p className="dv-hint">{t("tokenConsoleNote")}</p> : null}
      </div>
      <div className="dv-form-row">
        {token ? (
          <TextField id="dst-col-token" label={t("tokenId")} value={s.tokenID} onChange={s.setTokenID} placeholder="labtether@pve!monitoring" mono />
        ) : (
          <TextField id="dst-col-user" label={t("username")} value={s.username} onChange={s.setUsername} placeholder="root@pam" />
        )}
        <div className="dv-field">
          <label htmlFor="dst-col-secret">{token ? t("tokenSecret") : t("password")}</label>
          <input
            id="dst-col-secret"
            className="dv-input dv-mono"
            type="password"
            value={s.tokenSecret}
            onChange={(e) => s.setTokenSecret(e.target.value)}
            placeholder={keepSaved}
            autoComplete="new-password"
          />
        </div>
        <TextField
          id="dst-col-poll"
          label={t("poll")}
          type="number"
          min={15}
          max={3600}
          value={s.intervalSeconds}
          onChange={(value) => s.setIntervalSeconds(Number(value) || 60)}
        />
      </div>
      <OptionCheck checked={s.skipVerify} onChange={s.setSkipVerify} label={t("skipVerify")} warn={s.skipVerify} />
      <div className="dv-field">
        <label htmlFor="dst-col-ca">{t("ca")}</label>
        <textarea
          id="dst-col-ca"
          className="dv-input dv-mono dst-ca"
          rows={4}
          value={s.caPEM}
          onChange={(e) => s.setCAPEM(e.target.value)}
          placeholder="-----BEGIN CERTIFICATE-----"
          spellCheck={false}
        />
      </div>
      <div className="dv-form-acts">
        {s.message ? <span className="dv-ok-text dst-grow-l" role="status">{s.message}</span> : null}
        <Button size="sm" variant="ghost" onClick={onCancel} disabled={s.saving}>{t("cancel")}</Button>
        <Button size="sm" disabled={s.testing || s.saving} onClick={() => void s.testConnection()}>
          {s.testing ? t("testing") : t("test")}
        </Button>
        <Button size="sm" variant="primary" disabled={s.saving || s.testing} onClick={() => void s.save()}>
          {s.saving ? t("saving") : t("save")}
        </Button>
      </div>
    </div>
  );
}
