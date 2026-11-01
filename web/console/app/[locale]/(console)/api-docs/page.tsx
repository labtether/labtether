"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { ChevronDown, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { filterGroups, groupByTag, parseEndpoints, type OpenAPISpec } from "./apiDocsModel";
import { EndpointRow } from "./EndpointRow";

/** The hub's OpenAPI document, fetched once per visit (and again on Try again). */
function useSpec() {
  const [spec, setSpec] = useState<OpenAPISpec | null>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    fetch("/api/v2/openapi.json", { cache: "no-store" })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json() as Promise<OpenAPISpec>;
      })
      .then((data) => { if (!cancelled) { setSpec(data); setFailed(false); } })
      .catch(() => { if (!cancelled) setFailed(true); });
    return () => { cancelled = true; };
  }, [attempt]);
  const retry = useCallback(() => { setFailed(false); setAttempt((n) => n + 1); }, []);
  return { spec, failed, retry };
}

/**
 * API Docs (2.0): every endpoint of the hub's v2 API in one list, under its tag. A tag folds; a row opens to its
 * parameters, body and responses. The filter matches path, summary, method or tag.
 */
export default function ApiDocsPage() {
  const t = useTranslations("api-docs");
  const tn = useTranslations("nav");
  const { spec, failed, retry } = useSpec();
  const [query, setQuery] = useState("");
  const [closed, setClosed] = useState<Set<string>>(() => new Set());

  const endpoints = useMemo(() => parseEndpoints(spec), [spec]);
  const groups = useMemo(() => groupByTag(endpoints), [endpoints]);
  const shown = useMemo(() => filterGroups(groups, query), [groups, query]);
  const shownCount = [...shown.values()].reduce((n, eps) => n + eps.length, 0);
  const flip = (tag: string) => setClosed((current) => {
    const next = new Set(current);
    if (next.has(tag)) next.delete(tag); else next.add(tag);
    return next;
  });

  let body: ReactNode;
  if (failed) {
    body = (
      <div className="iu-empty">
        <b role="alert">{t("loadError")}</b>
        <Button onClick={retry}>{t("retry")}</Button>
      </div>
    );
  } else if (!spec) {
    body = <p role="status" className="iu-empty">{t("loading")}</p>;
  } else if (!shown.size) {
    body = <p className="iu-empty">{t("noResults", { query: query.trim() })}</p>;
  } else {
    body = [...shown.entries()].map(([tag, eps]) => {
      const isClosed = closed.has(tag) && !query.trim();
      return (
        <section key={tag} className="iu-api-sec" aria-label={tag}>
          <h2>
            <button type="button" className={`iu-gh${isClosed ? " closed" : ""}`} aria-expanded={!isClosed} onClick={() => flip(tag)}>
              <ChevronDown aria-hidden="true" strokeWidth={2.4} />
              {tag}
            </button>
          </h2>
          {!isClosed ? eps.map((ep) => <EndpointRow key={ep.id} endpoint={ep} />) : null}
        </section>
      );
    });
  }

  return (
    <>
      <PageHeader title={tn("apiDocs")} />
      <div className="iu-tools">
        <label className="iu-filter">
          <Search aria-hidden="true" strokeWidth={2} />
          <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("search")} aria-label={t("search")} />
        </label>
      </div>
      <p className="iu-api-auth">{t("authentication")}</p>
      <div className="iu-list iu-api">
        {body}
        {spec && query.trim() && shown.size ? (
          <div className="iu-foot">
            <span className="iu-num">{t("count", { shown: shownCount, total: endpoints.length })}</span>
            <button type="button" onClick={() => setQuery("")}>{t("clearFilter")}</button>
          </div>
        ) : null}
      </div>
    </>
  );
}
