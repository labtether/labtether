"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { parsePortInput } from "../../lib/portParsing";
import { CredentialProfileSelect, sshCredentialKinds } from "../credentials/CredentialProfileSelect";
import { useCredentialProfiles } from "../../hooks/useCredentialProfiles";
import { Button } from "../ui/Button";
import { Select } from "../ui/Input";
import { Modal } from "../ui/Modal";
import type { BookmarkFormData } from "./terminalSessionTypes";

type EditableBookmark = BookmarkFormData & { id: string };

type BookmarkDialogProps = {
  isOpen: boolean;
  onClose: () => void;
  onSave: (bookmark: BookmarkFormData) => void;
  editBookmark?: EditableBookmark;
  /** Devices a saved connection can point at; without them the form asks for an address only. */
  devices?: Array<{ id: string; name: string }>;
};

/** Save a connection: a name, then a device here or an address (host, port, user), and an optional saved login. */
export default function BookmarkDialog({ isOpen, onClose, onSave, editBookmark, devices = [] }: BookmarkDialogProps) {
  const t = useTranslations("terminal.bookmark");
  const [title, setTitle] = useState("");
  const [assetId, setAssetId] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("22");
  const [username, setUsername] = useState("");
  const [credentialProfileId, setCredentialProfileId] = useState("");
  const [titleError, setTitleError] = useState(false);
  const credentialProfiles = useCredentialProfiles(isOpen);
  const titleRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!isOpen) return undefined;
    setTitle(editBookmark?.title ?? "");
    setAssetId(editBookmark?.asset_id ?? "");
    setHost(editBookmark?.host ?? "");
    setPort(editBookmark?.port != null ? String(editBookmark.port) : "22");
    setUsername(editBookmark?.username ?? "");
    setCredentialProfileId(editBookmark?.credential_profile_id ?? "");
    setTitleError(false);
    const timer = window.setTimeout(() => titleRef.current?.focus(), 30);
    return () => window.clearTimeout(timer);
  }, [isOpen, editBookmark]);

  const linked = assetId.trim() !== "";

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const name = title.trim();
    if (!name) {
      setTitleError(true);
      titleRef.current?.focus();
      return;
    }
    const data: BookmarkFormData = { title: name };
    if (linked) data.asset_id = assetId.trim();
    else {
      if (host.trim()) data.host = host.trim();
      data.port = parsePortInput(port, 22);
      if (username.trim()) data.username = username.trim();
    }
    if (credentialProfileId.trim()) data.credential_profile_id = credentialProfileId.trim();
    onSave(data);
  };

  return (
    <Modal open={isOpen} onClose={onClose} title={editBookmark ? t("editTitle") : t("title")}>
      <form className="kt-dlg-body tm-form" onSubmit={handleSubmit} noValidate>
        <div className="kt-field tm-wide">
          <label htmlFor="tm-bm-title">{t("name")}</label>
          <input
            ref={titleRef}
            id="tm-bm-title"
            className="kt-input"
            value={title}
            placeholder="nas01"
            autoComplete="off"
            aria-invalid={titleError ? true : undefined}
            aria-describedby={titleError ? "tm-bm-title-err" : undefined}
            onChange={(event) => setTitle(event.target.value)}
          />
          {titleError ? <p className="kt-hint bad" id="tm-bm-title-err">{t("errName")}</p> : null}
        </div>
        <div className="kt-field tm-wide">
          <label htmlFor="tm-bm-device">{t("device")}</label>
          {devices.length > 0 || !assetId ? (
            <Select id="tm-bm-device" value={assetId} onChange={(event) => setAssetId(event.target.value)}>
              <option value="">{t("noDevice")}</option>
              {assetId && !devices.some((d) => d.id === assetId) ? <option value={assetId}>{assetId}</option> : null}
              {devices.map((device) => (
                <option key={device.id} value={device.id}>{device.name}</option>
              ))}
            </Select>
          ) : (
            <input id="tm-bm-device" className="kt-input" value={assetId} onChange={(event) => setAssetId(event.target.value)} />
          )}
        </div>
        {linked ? null : (
          <>
            <div className="kt-field">
              <label htmlFor="tm-bm-host">{t("host")}</label>
              <input id="tm-bm-host" className="kt-input tm-mono" value={host} placeholder="10.20.0.50" autoComplete="off" spellCheck={false} onChange={(event) => setHost(event.target.value)} />
            </div>
            <div className="kt-field">
              <label htmlFor="tm-bm-port">{t("port")}</label>
              <input id="tm-bm-port" className="kt-input tm-mono" inputMode="numeric" value={port} placeholder="22" onChange={(event) => setPort(event.target.value)} />
            </div>
            <div className="kt-field">
              <label htmlFor="tm-bm-user">{t("user")}</label>
              <input id="tm-bm-user" className="kt-input tm-mono" value={username} placeholder="root" autoComplete="off" spellCheck={false} onChange={(event) => setUsername(event.target.value)} />
            </div>
          </>
        )}
        <div className="kt-field tm-wide">
          <CredentialProfileSelect
            id="tm-bm-login"
            label={t("login")}
            value={credentialProfileId}
            onChange={setCredentialProfileId}
            profiles={credentialProfiles.profiles}
            loading={credentialProfiles.loading}
            error={credentialProfiles.error}
            allowedKinds={sshCredentialKinds}
            emptyLabel={t("noLogin")}
          />
        </div>
        <div className="kt-dlg-acts tm-wide">
          <Button type="button" variant="ghost" onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" variant="primary">{t("save")}</Button>
        </div>
      </form>
    </Modal>
  );
}
