"use client";

import { useCallback, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Copy, Plus } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { AddDeviceModal } from "../../../components/AddDeviceModal";
import { useAuth } from "../../../contexts/AuthContext";
import { useFastStatus } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { useLogs } from "../../../hooks/useLogs";
import { hasWriteRole } from "../../../lib/roles";
import type { LogEvent } from "../../../console/models";
import { EmptyLine, RowCheck } from "./LogControls";
import { LogLine, OpenLogLine, type LineCtx } from "./LogLine";
import { LogsToolbar } from "./LogsToolbar";
import { contextLines, lineText, logTime, newestFirst, selectedText } from "./logsModel";

/** Logs: one stream from every device, newest first. Filter, Device, Level, Range and Source; Live polls again. */
export default function LogsPage() {
  const t = useTranslations("logs");
  const locale = useLocale();
  const { user } = useAuth();
  const { addToast } = useToast();
  const fast = useFastStatus();
  const logs = useLogs();
  const [openId, setOpenId] = useState("");
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [addOpen, setAddOpen] = useState(false);

  const assets = useMemo(() => fast?.assets ?? [], [fast?.assets]);
  const names = useMemo(() => new Map(assets.map((a) => [a.id, a.name || a.id])), [assets]);
  const devices = useMemo(
    () => assets.map((a) => ({ value: a.id, label: a.name || a.id })).sort((a, b) => a.label.localeCompare(b.label)),
    [assets],
  );

  // A failed query falls back to the lines the status already carries, with the error above them.
  const shown = useMemo(
    () => newestFirst(logs.logsError && logs.logEvents.length === 0 ? logs.recentLogs : logs.logEvents),
    [logs.logEvents, logs.logsError, logs.recentLogs],
  );
  const ctx = useMemo<LineCtx>(() => {
    const now = new Date();
    return {
      timeOf: (event: LogEvent) => logTime(event.timestamp, now, locale),
      deviceOf: (event: LogEvent) => (event.asset_id ? names.get(event.asset_id) ?? event.asset_id : ""),
    };
  }, [locale, names]);
  const textOf = useCallback((event: LogEvent) => lineText(event, ctx.timeOf(event), ctx.deviceOf(event)), [ctx]);

  const copy = useCallback(
    (text: string, done: string) => {
      const fail = () => addToast("error", t("copyFailed"));
      try {
        void navigator.clipboard.writeText(text).then(() => addToast("success", done), fail);
      } catch {
        fail();
      }
    },
    [addToast, t],
  );

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const picked = shown.filter((e) => selected.has(e.id)).length;
  const allPicked = shown.length > 0 && picked === shown.length;

  const filtered =
    logs.logQuery.trim() !== "" || logs.selectedAsset !== "all" || logs.logLevel !== "all" || logs.selectedLogSource !== "all";
  const clearFilter = () => {
    logs.setLogQuery("");
    logs.setSelectedAsset("all");
    logs.setLogLevel("all");
    logs.setSelectedLogSource("all");
  };

  // "Nothing here yet" only when the hub has no lines from any source; otherwise the toolbar stays to widen the range.
  const firstRun =
    logs.logsLoaded && !logs.logsLoading && !logs.logsError && shown.length === 0 && !filtered &&
    logs.recentLogs.length === 0 && logs.logSources.length === 0;
  if (firstRun) {
    return (
      <>
        <PageHeader title={t("title")} />
        <EmptyLine line={t("empty")}>
          {assets.length === 0 && hasWriteRole(user?.role) ? (
            <Button variant="primary" onClick={() => setAddOpen(true)}>
              <Plus size={14} aria-hidden="true" />
              {t("addDevice")}
            </Button>
          ) : null}
        </EmptyLine>
        <AddDeviceModal open={addOpen} onClose={() => setAddOpen(false)} />
      </>
    );
  }

  const emptyText = logs.logQuery.trim()
    ? t("noMatch", { query: logs.logQuery.trim() })
    : filtered ? t("noneFiltered") : t("noneInRange");
  return (
    <>
      <PageHeader title={t("title")} />
      <LogsToolbar
        query={logs.logQuery}
        onQuery={logs.setLogQuery}
        devices={devices}
        device={logs.selectedAsset}
        onDevice={logs.setSelectedAsset}
        level={logs.logLevel}
        onLevel={logs.setLogLevel}
        windows={logs.telemetryWindows}
        window={logs.logWindow}
        onWindow={logs.setLogWindow}
        sources={logs.logSources}
        source={logs.selectedLogSource}
        onSource={logs.setSelectedLogSource}
        heartbeats={logs.includeHeartbeats}
        onHeartbeats={logs.setIncludeHeartbeats}
        live={logs.live}
        onLive={logs.setLive}
      />
      {logs.logsError ? (
        <div className="lg-err" role="alert">
          <span>{t("loadFailed")}</span>
          <Button size="sm" variant="ghost" onClick={logs.reload}>{t("retry")}</Button>
        </div>
      ) : null}
      {picked > 0 ? (
        <div className="lg-bulk" role="toolbar" aria-label={t("selected", { count: picked })}>
          <span className="lg-bulk-n">{t("selected", { count: picked })}</span>
          <Button size="sm" onClick={() => copy(selectedText(shown, selected, textOf), t("copiedLines", { count: picked }))}>
            <Copy size={14} aria-hidden="true" />
            {t("copyLines", { count: picked })}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>{t("clearSelection")}</Button>
        </div>
      ) : null}
      <div className={`lg-box${picked > 0 ? " picking" : ""}`} aria-busy={logs.logsLoading}>
        {shown.length > 0 ? (
          <div className="lg-ln lg-cols">
            <RowCheck
              checked={allPicked}
              mixed={picked > 0 && !allPicked}
              label={t("selectAll")}
              onChange={() => setSelected(allPicked ? new Set() : new Set(shown.map((e) => e.id)))}
            />
            <span>{t("cols.time")}</span>
            <span>{t("cols.level")}</span>
            <span>{t("cols.device")}</span>
            <span>{t("cols.source")}</span>
            <span>{t("cols.message")}</span>
          </div>
        ) : null}
        <div className="lg-rows">
          {shown.map((event) => {
            if (event.id !== openId) {
              return (
                <LogLine key={event.id} event={event} ctx={ctx} open={false} selected={selected.has(event.id)} onOpen={() => setOpenId(event.id)} onSelect={() => toggle(event.id)} />
              );
            }
            const { newer, older } = contextLines(shown, event);
            return (
              <OpenLogLine
                key={event.id}
                event={event}
                ctx={ctx}
                newer={newer}
                older={older}
                selected={selected.has(event.id)}
                onClose={() => setOpenId("")}
                onSelect={() => toggle(event.id)}
                onCopy={() => copy(textOf(event), t("copiedLine"))}
              />
            );
          })}
          {shown.length === 0 ? <p className="lg-none">{logs.logsLoading ? t("loading") : emptyText}</p> : null}
        </div>
        {filtered ? (
          <div className="lg-foot">
            <span>{t("lines", { count: shown.length })}</span>
            <button type="button" onClick={clearFilter}>{t("clearFilter")}</button>
          </div>
        ) : null}
      </div>
    </>
  );
}
