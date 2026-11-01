"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { parsePortInput } from "../../../lib/portParsing";
import { createBookmark, updateBookmark, type RemoteBookmark } from "./remoteBookmarksClient";
import { defaultPort, PROTOCOLS, type RemoteViewProtocol } from "./types";

/**
 * Add or edit a bookmark: name, protocol, host and port. A VNC or ARD bookmark needs plain VNC allowed before it can
 * be saved. editing: the bookmark being changed, or null to add one.
 */
export function BookmarkForm({
  editing,
  onSaved,
  onCancel,
}: {
  editing: RemoteBookmark | null;
  onSaved: (bookmark: RemoteBookmark) => void;
  onCancel: () => void;
}) {
  const t = useTranslations("desktop.bm");
  const [label, setLabel] = useState(editing?.label ?? "");
  const [host, setHost] = useState(editing?.host ?? "");
  const [protocol, setProtocol] = useState<RemoteViewProtocol>(editing?.protocol ?? "vnc");
  const [port, setPort] = useState(String(editing?.port ?? defaultPort("vnc")));
  const [allowPlain, setAllowPlain] = useState(!!editing?.allow_insecure_vnc);
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState(false);
  const plainVnc = protocol === "vnc" || protocol === "ard";
  const ready = !!host.trim() && !saving && (!plainVnc || allowPlain);

  const save = async () => {
    if (!ready) return;
    setSaving(true);
    setFailed(false);
    const body = {
      label: label.trim() || host.trim(),
      protocol,
      host: host.trim(),
      port: parsePortInput(port, defaultPort(protocol)),
      ...(plainVnc ? { allow_insecure_vnc: allowPlain } : {}),
    };
    try {
      onSaved(editing ? await updateBookmark(editing.id, body) : await createBookmark(body));
    } catch (err) {
      console.error("Failed to save bookmark:", err);
      setFailed(true);
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      className="rv-sheet rv-bm-form"
      aria-label={t(editing ? "editTitle" : "addTitle")}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") onCancel();
      }}
    >
      <div className="rv-grid bm">
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-bm-name">
            {t("name")}
          </label>
          <input id="rv-bm-name" className="rv-input" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="desk" autoFocus />
        </div>
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-bm-proto">
            {t("protocol")}
          </label>
          <select
            id="rv-bm-proto"
            className="rv-input"
            value={protocol}
            onChange={(e) => {
              const next = e.target.value as RemoteViewProtocol;
              if (port === "" || port === String(defaultPort(protocol))) setPort(String(defaultPort(next)));
              setProtocol(next);
              setAllowPlain(false);
            }}
          >
            {PROTOCOLS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </div>
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-bm-host">
            {t("host")}
          </label>
          <input
            id="rv-bm-host"
            className="rv-input mono"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="desk.example.com"
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-bm-port">
            {t("port")}
          </label>
          <input id="rv-bm-port" className="rv-input mono" inputMode="numeric" value={port} onChange={(e) => setPort(e.target.value)} />
        </div>
      </div>
      {plainVnc ? (
        <label className="rv-check top">
          <input type="checkbox" checked={allowPlain} onChange={(e) => setAllowPlain(e.target.checked)} />
          <span>
            {t("plainVnc")}
            <span className="rv-risk-t">{t("plainVncRisk")}</span>
          </span>
        </label>
      ) : null}
      <div className="rv-sheet-foot">
        {failed ? (
          <p className="rv-err" role="alert">
            {t("saveFailed")}
          </p>
        ) : null}
        <button type="button" className="rv-btn" onClick={onCancel}>
          {t("cancel")}
        </button>
        <button type="submit" className="rv-btn" disabled={!ready}>
          {t(saving ? "saving" : editing ? "save" : "add")}
        </button>
      </div>
    </form>
  );
}
