"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";

import { credentialKindLabels } from "../../../../components/credentials/CredentialProfileSelect";
import { Button } from "../../../../components/ui/Button";
import { type CredentialProfile, useCredentialProfiles } from "../../../../hooks/useCredentialProfiles";
import { CreateCredentialDialog, DeleteCredentialDialog, RotateCredentialDialog } from "./CredentialProfileDialogs";
import { RowMenu } from "./RowMenu";
import { Message, Section, Sheet, Status } from "./SettingsKit";
import { formatWhen } from "./settingsModel";

/**
 * Saved logins: the hub's encrypted SSH keys and passwords that devices, bookmarks and file connections use.
 * Each row: name, type and user; when its secret last changed; ⋯ Rotate… and Delete…. The hub's own SSH identity
 * is protected and has no actions here.
 */
export function CredentialProfilesCard() {
  const t = useTranslations("security");
  const locale = useLocale();
  const inventory = useCredentialProfiles();
  const [createOpen, setCreateOpen] = useState(false);
  const [rotateProfile, setRotateProfile] = useState<CredentialProfile | null>(null);
  const [deleteProfile, setDeleteProfile] = useState<CredentialProfile | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const mutationComplete = (nextMessage: string) => {
    setMessage(nextMessage);
    inventory.refresh();
  };

  const changed = (profile: CredentialProfile) => formatWhen(profile.rotated_at ?? profile.updated_at, new Date(), locale);

  return (
    <Section
      title={t("logins.title")}
      id="saved-logins"
      action={<Button size="sm" onClick={() => { setMessage(null); setCreateOpen(true); }}>{t("logins.add")}</Button>}
    >
      <Sheet className={inventory.loading && inventory.profiles.length === 0 ? "st-loading" : ""}>
        {inventory.profiles.length > 0 ? (
          <div className="st-row st-lg st-cols" aria-hidden="true">
            <span>{t("logins.colName")}</span>
            <span>{t("logins.colState")}</span>
            <span className="r">{t("logins.colChanged")}</span>
            <span />
          </div>
        ) : null}
        <ul aria-label={t("logins.title")}>
          {inventory.profiles.map((profile) => {
            const protectedIdentity = profile.kind === "hub_ssh_identity";
            const kind = credentialKindLabels[profile.kind] ?? profile.kind;
            return (
              <li key={profile.id} className="st-row st-lg">
                <div className="st-row-l">
                  <span className="st-nm">{profile.name}</span>
                  <span className="st-g" title={profile.description || undefined}>{profile.username ? `${kind} · ${profile.username}` : kind}</span>
                </div>
                <span className="st-cell">
                  {profile.status === "disabled" ? <Status tone="off">{t("logins.disabled")}</Status> : protectedIdentity ? t("logins.protected") : t("logins.active")}
                </span>
                <span className="st-cell st-num r c3">{changed(profile)}</span>
                <RowMenu
                  label={t("logins.actionsFor", { name: profile.name })}
                  items={protectedIdentity ? [] : [
                    { label: t("logins.rotate"), onSelect: () => { setMessage(null); setRotateProfile(profile); } },
                    { label: t("logins.delete"), danger: true, separatorBefore: true, onSelect: () => { setMessage(null); setDeleteProfile(profile); } },
                  ]}
                />
              </li>
            );
          })}
        </ul>
        {!inventory.loading && !inventory.error && inventory.profiles.length === 0 ? (
          <div className="st-empty">
            <b>{t("logins.empty")}</b>
            <span>{t("logins.emptyHint")}</span>
          </div>
        ) : null}
        {inventory.error ? (
          <div className="st-row" role="alert">
            <span className="st-msg bad">{inventory.error}</span>
            <div className="st-row-r">
              <Button size="sm" variant="ghost" onClick={inventory.refresh}>{t("retry")}</Button>
            </div>
          </div>
        ) : null}
        {message ? <Message>{message}</Message> : null}
      </Sheet>

      <CreateCredentialDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={() => mutationComplete(t("logins.added"))}
      />
      <RotateCredentialDialog
        key={`rotate-${rotateProfile?.id ?? "none"}`}
        profile={rotateProfile}
        onClose={() => setRotateProfile(null)}
        onRotated={() => mutationComplete(t("logins.rotated"))}
      />
      <DeleteCredentialDialog
        key={`delete-${deleteProfile?.id ?? "none"}`}
        profile={deleteProfile}
        onClose={() => setDeleteProfile(null)}
        onDeleted={() => mutationComplete(t("logins.deleted"))}
      />
    </Section>
  );
}
