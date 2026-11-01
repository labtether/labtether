"use client";

import { ChevronDown, ChevronUp } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { formatMem } from "../../../../components/containers/containerModel";
import { Bar, ListEmpty, ListError, ListSkeleton } from "../../../../components/containers/PanelParts";
import { imageRows, type ImageRow as Row } from "../../../../components/containers/stackModel";
import { fetchDockerContainers, fetchDockerImages, type DockerContainer } from "../../../../../lib/docker";

type ImageSort = { key: "name" | "size"; dir: "asc" | "desc" };

/** A Docker host's images: tag, size and the containers that use them, biggest first. */
export function DockerImagesTab({ hostId, bar }: { hostId: string; bar?: ReactNode }) {
  const t = useTranslations("devices-docker.images");
  const locale = useLocale();
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [sort, setSort] = useState<ImageSort>({ key: "size", dir: "desc" });

  useEffect(() => {
    let live = true;
    setLoading(true);
    void Promise.all([fetchDockerImages(hostId), fetchDockerContainers(hostId).catch(() => [] as DockerContainer[])])
      .then(([images, containers]) => { if (live) { setRows(imageRows(images, containers)); setError(null); } })
      .catch((err: unknown) => { if (live) setError(err instanceof Error ? err.message : "error"); })
      .finally(() => { if (live) setLoading(false); });
    return () => { live = false; };
  }, [hostId, attempt]);

  const sorted = useMemo(() => {
    const name = (r: Row) => r.tags[0] ?? "";
    return [...rows].sort((a, b) => {
      const d = sort.key === "size" ? a.size - b.size : name(a).localeCompare(name(b));
      return sort.dir === "asc" ? d : -d;
    });
  }, [rows, sort]);
  const date = (iso: string) => {
    const d = new Date(iso);
    return Number.isFinite(d.getTime()) ? d.toLocaleDateString(locale, { day: "numeric", month: "short", year: "numeric" }) : "";
  };
  const head = (key: ImageSort["key"], label: string, right = false) => {
    const on = sort.key === key;
    const Icon = on && sort.dir === "desc" ? ChevronDown : ChevronUp;
    return (
      <button type="button" className={`ct-sort${on ? " on" : ""}${right ? " r" : ""}`} aria-pressed={on}
        onClick={() => setSort((s) => (s.key === key ? { key, dir: s.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "size" ? "desc" : "asc" }))}>
        {label}
        {on ? <Icon aria-hidden="true" strokeWidth={2.2} /> : null}
      </button>
    );
  };

  let body: ReactNode;
  if (loading && !rows.length) body = <ListSkeleton />;
  else if (error && !rows.length) body = <ListError text={t("loadFailed")} onRetry={() => setAttempt((n) => n + 1)} />;
  else if (!rows.length) body = <ListEmpty text={t("none")} />;
  else body = (
    <div className="ct-list" role="table" aria-label={t("label")}>
      <div className="ct-row im ct-cols" role="row">
        <span className="ct-lead" />
        {head("name", t("image"))}
        {head("size", t("size"), true)}
        <span className="ct-names">{t("usedBy")}</span>
      </div>
      {sorted.map((r) => {
        const size = formatMem(r.size).split(" ");
        return (
          <div key={r.id || r.tags.join()} className="ct-row im" role="row">
            <span className="ct-lead" />
            <span className="ct-main" role="cell">
              <span className="ct-img mono" title={r.tags.join(", ") || r.id}>{r.tags.length ? r.tags.join(", ") : t("untagged", { id: r.id.slice(0, 12) })}</span>
              <span className="ct-grey">{[r.tags.length ? r.id.slice(0, 12) : "", date(r.created)].filter(Boolean).join(" · ")}</span>
            </span>
            <span className="ct-n" role="cell">{size[0]}<span className="u"> {size[1]}</span></span>
            <span className="ct-names plain" role="cell">
              {r.users.length ? r.users.join(", ") : <span className="ct-grey">{t("unused")}</span>}
            </span>
          </div>
        );
      })}
    </div>
  );

  return (
    <>
      <Bar left={bar} />
      {body}
    </>
  );
}
