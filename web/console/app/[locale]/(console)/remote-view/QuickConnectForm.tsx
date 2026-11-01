"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { QuickConnectLogin } from "./QuickConnectLogin";
import { canSaveDraft, draftFor, toConnectParams, withHostInput, withProtocol, type AdhocConnectParams, type QuickConnectDraft } from "./quickConnectModel";
import { defaultPort, PROTOCOLS } from "./types";

/**
 * Quick connect, opened from the page header: a sheet in two steps. 1 Connection: protocol, host and port, and whether
 * to keep it as a bookmark. 2 Login: a username and password, and the protocol's security choices. Esc closes it.
 */
export default function QuickConnectForm({
  initialHost = "",
  onClose,
  onConnect,
}: {
  initialHost?: string;
  onClose: () => void;
  onConnect: (params: AdhocConnectParams) => void;
}) {
  const t = useTranslations("desktop.qc");
  const [step, setStep] = useState<1 | 2>(1);
  const [draft, setDraft] = useState<QuickConnectDraft>(() => draftFor(initialHost));
  const update = (patch: Partial<QuickConnectDraft>) => setDraft((d) => ({ ...d, ...patch }));

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const connect = (withLogin: boolean) => {
    onConnect(toConnectParams(draft, withLogin));
    onClose();
  };
  const canSave = canSaveDraft(draft);

  return (
    <section className="rv-sheet rv-qc" aria-label={t("title")}>
      <p className="rv-qc-steps">
        <span className={step === 1 ? "on" : ""}>{t("step1")}</span>
        <span className={step === 2 ? "on" : ""}>{t("step2")}</span>
      </p>
      {step === 1 ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (draft.host.trim()) setStep(2);
          }}
        >
          <div className="rv-grid qc">
            <div className="rv-field">
              <span className="rv-field-l" id="rv-qc-proto">
                {t("protocol")}
              </span>
              <div className="rv-seg" role="group" aria-labelledby="rv-qc-proto">
                {PROTOCOLS.map((p) => (
                  <button
                    key={p.value}
                    type="button"
                    className={draft.protocol === p.value ? "on" : ""}
                    aria-pressed={draft.protocol === p.value}
                    onClick={() => setDraft((d) => withProtocol(d, p.value))}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <div className="rv-field">
              <label className="rv-field-l" htmlFor="rv-qc-host">
                {t("host")}
              </label>
              <input
                id="rv-qc-host"
                className="rv-input mono"
                value={draft.host}
                onChange={(event) => setDraft((d) => withHostInput(d, event.target.value))}
                placeholder="desk.example.com"
                autoComplete="off"
                spellCheck={false}
                autoFocus
                aria-describedby="rv-qc-host-hint"
              />
              <p className="rv-hint" id="rv-qc-host-hint">
                {t("hostHint")}
              </p>
            </div>
            <div className="rv-field">
              <label className="rv-field-l" htmlFor="rv-qc-port">
                {t("port")}
              </label>
              <input
                id="rv-qc-port"
                className="rv-input mono"
                inputMode="numeric"
                value={draft.port}
                onChange={(event) => update({ port: event.target.value })}
                placeholder={String(defaultPort(draft.protocol))}
              />
            </div>
          </div>
          <label className={`rv-check${canSave ? "" : " off"}`}>
            <input
              type="checkbox"
              checked={draft.saveBookmark}
              disabled={!canSave}
              onChange={(event) => update({ saveBookmark: event.target.checked })}
            />
            {t(canSave ? "saveBookmark" : "cannotSave")}
          </label>
          {draft.saveBookmark ? (
            <div className="rv-field rv-qc-name">
              <label className="rv-field-l" htmlFor="rv-qc-name">
                {t("name")}
              </label>
              <input
                id="rv-qc-name"
                className="rv-input"
                value={draft.bookmarkName}
                onChange={(event) => update({ bookmarkName: event.target.value })}
                placeholder={draft.host.trim() || "desk"}
              />
            </div>
          ) : null}
          <div className="rv-sheet-foot">
            <button type="button" className="rv-btn" onClick={onClose}>
              {t("cancel")}
            </button>
            <button type="submit" className="rv-btn primary" disabled={!draft.host.trim()}>
              {t("next")}
            </button>
          </div>
        </form>
      ) : (
        <QuickConnectLogin draft={draft} setDraft={setDraft} onBack={() => setStep(1)} onCancel={onClose} onConnect={connect} />
      )}
    </section>
  );
}
