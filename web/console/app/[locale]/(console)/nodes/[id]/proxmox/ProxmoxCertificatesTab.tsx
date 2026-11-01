"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { withCollector } from "../proxmoxFormatters";
import { DiCols, DiEmpty, DiKV, DiList, DiMain, DiNote, DiRow, DiSection, DiStatus } from "./DiList";
import { daysUntil, whenText } from "./diFormat";
import { useProxmoxList } from "./useProxmoxData";

type Certificate = {
  filename?: string;
  fingerprint?: string;
  issuer?: string;
  subject?: string;
  notbefore?: number;
  notafter?: number;
  public_key_bits?: number;
  public_key_type?: string;
  san?: string[];
};

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

/** The node's certificates with their expiry: amber within 30 days, red within 7 or once expired. A row opens it. */
export function ProxmoxCertificatesTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const t = useTranslations("devices-proxmox.pve.certificates");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [open, setOpen] = useState<string | null>(null);
  const path = proxmoxNode
    ? withCollector(`/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/certificates`, proxmoxCollectorID)
    : null;
  const { data: certs, loading, error, refresh } = useProxmoxList<Certificate>(path);

  return (
    <DiSection
      title={t("title")}
      right={<Button size="sm" onClick={refresh} disabled={loading}>{loading ? kit("loading") : kit("refresh")}</Button>}
    >
      {error ? <DiNote tone="bad">{t("loadFailed", { error })}</DiNote> : null}
      <DiList cols="minmax(0, 1fr) 200px" colsS="minmax(0, 1fr) auto" label={t("title")}>
        <DiCols>
          <span>{t("certificate")}</span>
          <span className="r">{t("expires")}</span>
        </DiCols>
        {certs.length === 0 ? <DiEmpty title={loading ? kit("loading") : t("none")} /> : null}
        {certs.map((cert, index) => {
          const key = cert.fingerprint ?? String(index);
          const days = daysUntil(cert.notafter);
          const when = whenText(cert.notafter, locale);
          const tone = days == null ? "" : days < 7 ? "bad" : days < 30 ? "warn" : "";
          const word = days == null ? "—" : days < 0 ? t("expired", { when }) : t("expiresIn", { days, when });
          const isOpen = open === key;
          return (
            <DiRow
              key={key}
              tone={tone}
              onOpen={() => setOpen(isOpen ? null : key)}
              selected={isOpen}
              after={isOpen ? (
                <div className="di-panel">
                  <DiKV
                    items={[
                      [t("subject"), cert.subject || null],
                      [t("issuer"), cert.issuer || null],
                      [t("validFrom"), whenText(cert.notbefore, locale) || null],
                      [t("key"), cert.public_key_type ? `${cert.public_key_type}${cert.public_key_bits != null ? ` ${cert.public_key_bits}` : ""}` : null],
                      [t("fingerprint"), cert.fingerprint ? <span className="di-mono">{cert.fingerprint}</span> : null],
                      [t("names"), cert.san && cert.san.length > 0 ? cert.san.join(", ") : null],
                    ]}
                  />
                </div>
              ) : null}
            >
              <DiMain name={cert.filename || t("unnamed", { n: index + 1 })} grey={cert.subject || null} />
              {tone ? <DiStatus tone={tone} word={word} /> : <span className="di-num">{word}</span>}
            </DiRow>
          );
        })}
      </DiList>
    </DiSection>
  );
}
