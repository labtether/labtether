"use client";

import type { Dispatch, SetStateAction } from "react";
import { useTranslations } from "next-intl";
import { canSaveDraft, connectBlocked, type QuickConnectDraft } from "./quickConnectModel";

type Props = {
  draft: QuickConnectDraft;
  setDraft: Dispatch<SetStateAction<QuickConnectDraft>>;
  onBack: () => void;
  onCancel: () => void;
  onConnect: (withLogin: boolean) => void;
};

/**
 * Quick connect step 2: the address being opened, a username and password, and the protocol's security choices. The
 * RDP choices exclude one another and are for this session only, so choosing one turns off Save as bookmark.
 */
export function QuickConnectLogin({ draft, setDraft, onBack, onCancel, onConnect }: Props) {
  const t = useTranslations("desktop.qc");
  const update = (patch: Partial<QuickConnectDraft>) => setDraft((d) => ({ ...d, ...patch }));
  const blocked = connectBlocked(draft);
  const address = `${draft.protocol}://${draft.host.trim()}:${draft.port}`;

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!blocked) onConnect(true);
      }}
    >
      <p className="rv-qc-sum">
        <span className="mono">{address}</span>
        <button type="button" className="rv-link" onClick={onBack}>
          {t("edit")}
        </button>
      </p>
      <div className="rv-grid">
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-qc-user">
            {t("username")}
          </label>
          <input
            id="rv-qc-user"
            className="rv-input mono"
            value={draft.username}
            onChange={(event) => update({ username: event.target.value })}
            autoComplete="username"
            spellCheck={false}
            autoFocus
          />
        </div>
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-qc-pass">
            {t("password")}
          </label>
          <input
            id="rv-qc-pass"
            className="rv-input"
            type="password"
            value={draft.password}
            onChange={(event) => update({ password: event.target.value })}
            autoComplete="current-password"
          />
        </div>
      </div>

      {draft.protocol === "rdp" ? (
        <div className="rv-risk">
          <div className="rv-field">
            <label className="rv-field-l" htmlFor="rv-qc-fp">
              {t("fingerprint")}
            </label>
            <input
              id="rv-qc-fp"
              className="rv-input mono"
              value={draft.certificateFingerprints}
              placeholder="sha256:AA:BB:…"
              autoComplete="off"
              aria-describedby="rv-qc-fp-hint"
              onChange={(event) => {
                const value = event.target.value;
                update(value.trim() ? { certificateFingerprints: value, saveBookmark: false, ignoreCertificate: false, allowLegacySecurity: false } : { certificateFingerprints: value });
              }}
            />
            <p className="rv-hint" id="rv-qc-fp-hint">
              {t("fingerprintHint")}
            </p>
          </div>
          <RiskCheck
            checked={draft.ignoreCertificate}
            label={t("ignoreCert")}
            risk={t("ignoreCertRisk")}
            onChange={(on) => update(on ? { ignoreCertificate: true, saveBookmark: false, allowLegacySecurity: false, certificateFingerprints: "" } : { ignoreCertificate: false })}
          />
          <RiskCheck
            checked={draft.allowLegacySecurity}
            label={t("legacyRdp")}
            risk={t("legacyRdpRisk")}
            onChange={(on) => update(on ? { allowLegacySecurity: true, saveBookmark: false, ignoreCertificate: false, certificateFingerprints: "" } : { allowLegacySecurity: false })}
          />
          {canSaveDraft(draft) ? null : <p className="rv-hint">{t("sessionOnly")}</p>}
        </div>
      ) : null}

      {draft.protocol === "vnc" || draft.protocol === "ard" ? (
        <div className="rv-risk">
          <RiskCheck checked={draft.allowInsecureVNC} label={t("plainVnc")} risk={t("plainVncRisk")} onChange={(on) => update({ allowInsecureVNC: on })} />
          {draft.allowInsecureVNC ? null : <p className="rv-hint">{t("plainVncNeeded")}</p>}
        </div>
      ) : null}

      {draft.protocol === "spice" ? (
        <div className="rv-risk">
          <RiskCheck
            checked={draft.spiceSecurityMode === "cleartext"}
            label={t("plainSpice")}
            risk={t("plainSpiceRisk")}
            onChange={(on) => update(on ? { spiceSecurityMode: "cleartext", spiceCAPEM: "" } : { spiceSecurityMode: "tls" })}
          />
          {draft.spiceSecurityMode === "tls" ? (
            <div className="rv-field">
              <label className="rv-field-l" htmlFor="rv-qc-ca">
                {t("caCert")}
              </label>
              <textarea
                id="rv-qc-ca"
                className="rv-input mono rv-ta"
                value={draft.spiceCAPEM}
                maxLength={16 * 1024}
                rows={4}
                spellCheck={false}
                placeholder="-----BEGIN CERTIFICATE-----"
                aria-describedby="rv-qc-ca-hint"
                onChange={(event) => update({ spiceCAPEM: event.target.value })}
              />
              <p className="rv-hint" id="rv-qc-ca-hint">
                {t("caHint")}
              </p>
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="rv-sheet-foot">
        <button type="button" className="rv-btn" onClick={onCancel}>
          {t("cancel")}
        </button>
        <button type="button" className="rv-btn" disabled={blocked} onClick={() => onConnect(false)}>
          {t("connectNoLogin")}
        </button>
        <button type="submit" className="rv-btn primary" disabled={blocked}>
          {t("connect")}
        </button>
      </div>
    </form>
  );
}

/** A security choice: the checkbox, then what it exposes in the warning colour. */
function RiskCheck({ checked, label, risk, onChange }: { checked: boolean; label: string; risk: string; onChange: (on: boolean) => void }) {
  return (
    <label className="rv-check top">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span>
        {label}
        <span className="rv-risk-t">{risk}</span>
      </span>
    </label>
  );
}
