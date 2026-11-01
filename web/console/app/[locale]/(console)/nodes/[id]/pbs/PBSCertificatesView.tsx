"use client";

// Certificates: each certificate file with its subject and expiry (amber within 30 days, red within 7 or once
// expired). A row opens its issuer, fingerprint and names.

import { useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { DiCols, DiEmpty, DiKV, DiList, DiMain, DiNote, DiRow, DiSection, DiStatus, DiTime } from "../proxmox/DiList";
import { daysUntil } from "../proxmox/diFormat";
import { certificateTone, normalizeCertificates, type PBSCertificate } from "./pbsJobsModel";
import { pbsErrorDetail, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY: PBSCertificate[] = [];
const COLS = "minmax(0, 1fr) minmax(120px, auto)";
const COLS_S = "minmax(0, 1fr) minmax(96px, auto)";

export function PBSCertificatesView({ assetId }: { assetId: string }) {
  const { t, tk, when } = usePBSWords();
  const certs = usePBSResource(`/api/pbs/assets/${encodeURIComponent(assetId)}/certificates`, normalizeCertificates, EMPTY);
  const [open, setOpen] = useState<string | null>(null);

  const expiry = (cert: PBSCertificate) => {
    if (!cert.not_after) return <DiTime className="quiet">—</DiTime>;
    const tone = certificateTone(cert.not_after);
    const days = daysUntil(cert.not_after) ?? 0;
    const word = days < 0 ? t("certs.expired", { time: when(cert.not_after) }) : tone ? t("certs.expiresIn", { count: days }) : when(cert.not_after);
    return tone ? <span className="di-pbs-last"><DiStatus tone={tone} word={word} /></span> : <DiTime>{word}</DiTime>;
  };

  return (
    <DiSection
      id="pbs-certificates"
      title={t("view.certificates")}
      right={<Button size="sm" variant="ghost" onClick={certs.refresh} disabled={certs.loading}>{tk("refresh")}</Button>}
    >
      {certs.error ? <DiNote tone="bad">{[t("certs.loadFailed"), pbsErrorDetail(certs.error)].filter(Boolean).join(" ")}</DiNote> : null}
      <DiList cols={COLS} colsS={COLS_S} hang label={t("view.certificates")}>
        <DiCols>
          <span>{t("cols.certificate")}</span>
          <span className="r">{t("cols.expires")}</span>
        </DiCols>
        {certs.data.length === 0 ? <DiEmpty title={certs.loaded ? t("certs.none") : tk("loading")} /> : null}
        {certs.data.map((cert) => {
          const expanded = open === cert.filename;
          const tone = certificateTone(cert.not_after);
          return (
            <DiRow
              key={cert.filename}
              tone={tone}
              onOpen={() => setOpen(expanded ? null : cert.filename)}
              selected={expanded}
              after={expanded ? (
                <div className="di-pbs-panel">
                  <DiKV
                    items={[
                      [t("kv.subject"), cert.subject || ""],
                      [t("kv.issuer"), cert.issuer || ""],
                      [t("kv.fingerprint"), cert.fingerprint ? <span className="di-mono">{cert.fingerprint}</span> : ""],
                      [t("kv.names"), cert.san?.join(", ") || ""],
                    ]}
                  />
                </div>
              ) : null}
            >
              <DiMain name={cert.filename} grey={cert.subject} />
              {expiry(cert)}
            </DiRow>
          );
        })}
      </DiList>
    </DiSection>
  );
}
