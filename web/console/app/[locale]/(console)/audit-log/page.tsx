"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { ChevronDown, ChevronUp } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { useAuth } from "../../../contexts/AuthContext";
import { useSlowStatus, useStatusControls } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { useDocumentVisibility } from "../../../hooks/useDocumentVisibility";
import { useHubUsers } from "../../../hooks/useHubUsers";
import { downloadCSV } from "../../../lib/export";
import { hasAdminRole } from "../../../lib/roles";
import { ChipGroup, EmptyLine, FilterField, PickMenu } from "../logs/LogControls";
import { AuditRow, type AuditText } from "./AuditRow";
import { AuditViews } from "./AuditViews";
import { actionKey, ALL_REQUESTS, auditRows, auditTime, humanizeType, outcome, peopleByActivity, REQUEST_TYPE, targetText, visibleAuditRows, type AuditEntry, type AuditSort } from "./auditModel";
import { copyText } from "../../../lib/clipboard";
import { useAuditPages } from "./useAuditPages";

const AUTO_REFRESH_MS = 30_000;

/** Audit log: who did what and when, newest first. Refused attempts read "Denied". Export CSV; Recordings beside it. */
export default function AuditLogPage() {
  const t = useTranslations("audit-log");
  const locale = useLocale();
  const { user } = useAuth();
  const { addToast } = useToast();
  const slow = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const visible = useDocumentVisibility();
  const { users } = useHubUsers(hasAdminRole(user?.role));
  const [query, setQuery] = useState("");
  const [who, setWho] = useState("all");
  const [type, setType] = useState("all");
  const [sort, setSort] = useState<AuditSort>("time:desc");
  const [openId, setOpenId] = useState("");

  // Everyone pages through the rows the Hub lets them read: the whole log for the owner and admins, their own rows
  // for everyone else (the Hub's audit list applies that rule in its query). visibleAuditRows is a second check
  // only; the Hub is what decides who may read a row.
  const wholeLog = hasAdminRole(user?.role);
  const pages = useAuditPages(Boolean(user?.id));
  const pageRows = useMemo(() => visibleAuditRows(pages.entries, wholeLog, user?.id), [pages.entries, user?.id, wholeLog]);
  const paged = pageRows.length > 0;
  const entries = useMemo(() => (paged ? pageRows : (slow?.recentAudit ?? []) as AuditEntry[]), [paged, pageRows, slow?.recentAudit]);
  const [exporting, setExporting] = useState(false);
  const { refresh: refreshPages } = pages;

  useEffect(() => {
    if (!visible) return;
    const id = window.setInterval(() => {
      void fetchStatus();
      void refreshPages();
    }, AUTO_REFRESH_MS);
    return () => window.clearInterval(id);
  }, [fetchStatus, refreshPages, visible]);

  // Only admins load the user list; everyone else still sees their own name on their own rows.
  const usernames = useMemo(() => {
    const names = new Map(users.map((u) => [u.id, u.username]));
    if (user?.id && user.username && !names.has(user.id)) names.set(user.id, user.username);
    return names;
  }, [user?.id, user?.username, users]);
  const nameOf = useCallback(
    (id: string) => (id === "system" ? t("hub") : usernames.get(id) ?? id),
    [t, usernames],
  );
  const text = useMemo<AuditText>(() => {
    const now = new Date();
    return {
      time: (e) => auditTime(e.timestamp, now, locale),
      who: (e) => nameOf(e.actor_id ?? "") || t("unknown"),
      action: (e) => (actionKey(e.type) ? t(`actions.${actionKey(e.type)}`) : humanizeType(e.type)),
    };
  }, [locale, nameOf, t]);
  const searchText = useCallback(
    (e: AuditEntry) => {
      const result = outcome(e.decision);
      return [text.time(e), text.who(e), text.action(e), targetText(e), e.target ?? "", e.type, result ? t(`outcome.${result}`) : ""].join(" ");
    },
    [t, text],
  );

  const people = useMemo(() => peopleByActivity(entries, (id) => text.who({ actor_id: id } as AuditEntry)), [entries, text]);
  const types = useMemo(
    () => [...new Set(entries.map((e) => e.type))].map((v) => ({ value: v, label: text.action({ type: v } as AuditEntry) })).sort((a, b) => a.label.localeCompare(b.label)),
    [entries, text],
  );
  const person = people.includes(who) ? who : "all";
  const hasRequests = types.some((o) => o.value === REQUEST_TYPE);
  const kind = types.some((o) => o.value === type) || (type === ALL_REQUESTS && hasRequests) ? type : "all";
  // API requests repeat the named changes beside them, so they show only when asked for.
  const total = kind === ALL_REQUESTS || kind === REQUEST_TYPE ? entries.length : entries.filter((e) => e.type !== REQUEST_TYPE).length;
  const rows = useMemo(
    () => auditRows(entries, { query, who: person, type: kind, sort, nameOf: (id) => text.who({ actor_id: id } as AuditEntry), textOf: searchText }),
    [entries, kind, person, query, searchText, sort, text],
  );

  // The export covers every row the filters match, not only the loaded pages.
  const exportCSV = async () => {
    let list = rows;
    if (paged) {
      setExporting(true);
      try {
        const all = visibleAuditRows(await pages.readAll(), wholeLog, user?.id);
        list = auditRows(all, { query, who: person, type: kind, sort, nameOf: (id) => text.who({ actor_id: id } as AuditEntry), textOf: searchText });
      } catch {
        addToast("error", t("exportFailed"));
        return;
      } finally {
        setExporting(false);
      }
    }
    downloadCSV(
      list.map((e) => ({
        timestamp: e.timestamp,
        person: text.who(e),
        action: text.action(e),
        target: targetText(e),
        target_id: e.target ?? "",
        result: e.decision ?? "",
        type: e.type,
        actor_id: e.actor_id ?? "",
        reason: e.reason ?? "",
        details: e.details ? JSON.stringify(e.details) : "",
      })),
      `labtether-audit-${new Date().toISOString().slice(0, 10)}.csv`,
    );
  };
  const copy = (line: string) => {
    const fail = () => addToast("error", t("copyFailed"));
    void copyText(line).then(() => addToast("success", t("copied")), fail);
  };
  const sortBy = (key: "time" | "who") =>
    setSort((cur) => (cur.startsWith(key) ? (cur.endsWith("desc") ? `${key}:asc` : `${key}:desc`) : key === "time" ? "time:desc" : "who:asc") as AuditSort);
  const sortHead = (key: "time" | "who", label: string) => {
    const on = sort.startsWith(key);
    const Arrow = sort.endsWith("asc") ? ChevronUp : ChevronDown;
    return (
      <button type="button" className={`lg-sort${on ? " on" : ""}`} onClick={() => sortBy(key)}>
        {label}
        {on ? <Arrow aria-hidden="true" strokeWidth={2} /> : null}
      </button>
    );
  };
  const filtered = rows.length < total;

  return (
    <>
      <PageHeader
        title={t("title")}
        action={<Button onClick={() => void exportCSV()} disabled={rows.length === 0 || exporting}>{exporting ? t("exporting") : t("exportCSV")}</Button>}
      />
      <AuditViews current="log" />
      {entries.length === 0 ? (
        <EmptyLine line={t("empty")} />
      ) : (
        <>
          <div className="lg-tools wide">
            <FilterField value={query} onChange={setQuery} placeholder={t("filter")} />
            {people.length > 1 ? (
              <ChipGroup label={t("cols.person")} value={person} options={[{ value: "all", label: t("all") }, ...people.map((p) => ({ value: p, label: p }))]} onChange={setWho} />
            ) : null}
            {types.length > 1 || hasRequests ? (
              <PickMenu
                label={t("cols.action")}
                value={kind}
                options={[{ value: "all", label: t("allActions") }, ...(hasRequests ? [{ value: ALL_REQUESTS, label: t("allRequests") }] : []), ...types]}
                onChange={setType}
              />
            ) : null}
          </div>
          <div className="lg-box lg-au-box">
            <div className="lg-au lg-au-cols">
              {sortHead("time", t("cols.time"))}
              {sortHead("who", t("cols.person"))}
              <span>{t("cols.action")}</span>
              <span />
            </div>
            {rows.map((e) => (
              <AuditRow key={e.id} entry={e} text={text} open={openId === e.id} onToggle={() => setOpenId(openId === e.id ? "" : e.id)} onCopy={copy} />
            ))}
            {rows.length === 0 ? <p className="lg-none">{query.trim() ? t("noMatch", { query: query.trim() }) : t("noneFiltered")}</p> : null}
            {filtered ? (
              <div className="lg-foot">
                <span>{t("shown", { count: rows.length, total })}</span>
                <button type="button" onClick={() => { setQuery(""); setWho("all"); setType("all"); }}>{t("clearFilter")}</button>
              </div>
            ) : null}
            {paged && pages.more ? (
              <div className="lg-foot">
                <span />
                <button type="button" onClick={() => void pages.loadMore()} disabled={pages.loading}>{pages.loading ? t("loadingMore") : t("loadMore")}</button>
              </div>
            ) : null}
          </div>
        </>
      )}
    </>
  );
}
