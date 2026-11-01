"use client";

import { useEffect, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { ChevronDown, ChevronUp, Download } from "lucide-react";
import { Link } from "../../../../i18n/navigation";
import { PageHeader } from "../../../components/PageHeader";
import { OperatorAccessRequired } from "../../../components/OperatorAccessRequired";
import { useAuth } from "../../../contexts/AuthContext";
import { useFastStatus } from "../../../contexts/StatusContext";
import { canAccessHostData } from "../../../lib/roles";
import { auditTime } from "../audit-log/auditModel";
import { AuditViews } from "../audit-log/AuditViews";
import { ChipGroup, EmptyLine, FilterField } from "../logs/LogControls";
import { durationText, recordingDownloadHref, recordingRows, sizeText, type Recording, type RecordingSort } from "./recordingsModel";

export default function RecordingsPage() {
  const t = useTranslations("recordings");
  const { user } = useAuth();
  // The Hub serves recordings (list and files) to operators and above only.
  if (!canAccessHostData(user?.role)) {
    return <OperatorAccessRequired title={t("title")} feature="recordings" />;
  }
  return <RecordingsList />;
}

/** Recordings: the Audit log's second view. Remote View sessions the hub recorded, newest first, each with Download. */
function RecordingsList() {
  const t = useTranslations("recordings");
  const ta = useTranslations("audit-log");
  const locale = useLocale();
  const fast = useFastStatus();
  const [recordings, setRecordings] = useState<Recording[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [query, setQuery] = useState("");
  const [protocol, setProtocol] = useState("all");
  const [sort, setSort] = useState<RecordingSort>("when:desc");

  useEffect(() => {
    const controller = new AbortController();
    fetch("/api/recordings", { cache: "no-store", signal: controller.signal })
      .then((response) => response.json())
      .then((payload: { recordings?: Recording[] }) => {
        setRecordings(Array.isArray(payload.recordings) ? payload.recordings : []);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setFailed(true);
        setRecordings([]);
      });
    return () => controller.abort();
  }, []);

  const names = useMemo(() => new Map((fast?.assets ?? []).map((a) => [a.id, a.name || a.id])), [fast?.assets]);
  const nameOf = (id: string) => names.get(id) ?? id;
  const list = useMemo(() => recordings ?? [], [recordings]);
  const protocols = useMemo(() => [...new Set(list.map((r) => r.protocol.toLowerCase()).filter(Boolean))].sort(), [list]);
  const chip = protocols.includes(protocol) ? protocol : "all";
  const rows = recordingRows(list, { query, protocol: chip, sort, nameOf });
  const now = new Date();

  const head = (key: "dev" | "dur" | "size" | "when", label: string, right = false) => {
    const on = sort.startsWith(`${key}:`);
    const Arrow = sort.endsWith("asc") ? ChevronUp : ChevronDown;
    const next = (on ? (sort.endsWith("desc") ? `${key}:asc` : `${key}:desc`) : key === "dev" ? "dev:asc" : `${key}:desc`) as RecordingSort;
    return (
      <button type="button" className={`lg-sort${on ? " on" : ""}${right ? " r" : ""}`} onClick={() => setSort(next)}>
        {label}
        {on ? <Arrow aria-hidden="true" strokeWidth={2} /> : null}
      </button>
    );
  };

  return (
    <>
      <PageHeader title={ta("title")} />
      <AuditViews current="recordings" />
      {recordings !== null && list.length === 0 ? (
        <EmptyLine line={failed ? t("loadFailed") : t("empty")} />
      ) : (
        <>
          <div className="lg-tools wide">
            <FilterField value={query} onChange={setQuery} placeholder={t("filter")} />
            {protocols.length > 1 ? (
              <ChipGroup
                label={t("columns.protocol")}
                value={chip}
                options={[{ value: "all", label: t("all") }, ...protocols.map((p) => ({ value: p, label: p.toUpperCase() }))]}
                onChange={setProtocol}
              />
            ) : null}
          </div>
          <div className="lg-box">
            <div className="lg-rc lg-rc-cols">
              {head("dev", t("columns.device"))}
              {head("dur", t("columns.duration"), true)}
              {head("size", t("columns.size"), true)}
              {head("when", t("columns.date"), true)}
              <span />
            </div>
            {rows.map((r) => {
              const href = recordingDownloadHref(r);
              const name = nameOf(r.asset_id);
              const live = r.status === "recording";
              return (
                <div key={r.id} className="lg-rc lg-rc-r">
                  <div className="lg-rc-nm">
                    <Link href={`/nodes/${encodeURIComponent(r.asset_id)}`} className="lg-rc-dev">{name}</Link>
                    <span className="lg-rc-sub">{t("kind", { protocol: r.protocol.toUpperCase() })}</span>
                  </div>
                  <span className="lg-rc-n">{live ? "" : durationText(r.duration_ms, locale)}</span>
                  <span className="lg-rc-n">{sizeText(r.file_size, locale)}</span>
                  <time className="lg-rc-n" dateTime={r.created_at}>{auditTime(r.created_at, now, locale)}</time>
                  {live ? (
                    <span className="lg-rc-live"><i className="lg-on" aria-hidden="true" />{t("inProgress")}</span>
                  ) : href ? (
                    <a href={href} download className="lg-btn quiet lg-rc-dl" aria-label={t("downloadLabel", { asset: name })}>
                      <Download aria-hidden="true" strokeWidth={1.75} />
                      <span>{t("download")}</span>
                    </a>
                  ) : <span />}
                </div>
              );
            })}
            {recordings === null ? <p className="lg-none">{t("loading")}</p> : null}
            {recordings !== null && rows.length === 0 ? <p className="lg-none">{t("noMatch", { query: query.trim() })}</p> : null}
            {rows.length < list.length ? (
              <div className="lg-foot">
                <span>{t("shown", { count: rows.length, total: list.length })}</span>
                <button type="button" onClick={() => { setQuery(""); setProtocol("all"); }}>{t("clearFilter")}</button>
              </div>
            ) : null}
          </div>
        </>
      )}
    </>
  );
}
