"use client";

// One task opened under its row: node, state, exit status, the log, Refresh, and Stop task (after a confirm) while
// it runs. A failed stop keeps the panel and the button as they were.

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { useDiConfirm } from "../proxmox/DiControls";
import { DiKV, DiLog, DiNote } from "../proxmox/DiList";
import {
  normalizePBSTaskLogLines,
  normalizePBSTaskStatus,
  type PBSTask,
  type PBSTaskLogLine,
  type PBSTaskStatus,
} from "../pbsTabModel";
import { taskFinished } from "./pbsViewModel";
import { pbsErrorDetail } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

type Props = {
  task: PBSTask;
  collectorID: string;
  nodeFallback: string;
  onStopped: () => void;
};

export function PBSTaskPanel({ task, collectorID, nodeFallback, onStopped }: Props) {
  const { t, tk, when, kindLabel } = usePBSWords();
  const { ask, dialog } = useDiConfirm();
  const [status, setStatus] = useState<PBSTaskStatus | null>(null);
  const [log, setLog] = useState<PBSTaskLogLine[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [stopping, setStopping] = useState(false);
  const [stopped, setStopped] = useState(false);
  const seqRef = useRef(0);

  const node = task.node?.trim() || nodeFallback;
  const upid = task.upid?.trim() ?? "";
  const query = collectorID ? `?collector_id=${encodeURIComponent(collectorID)}` : "";
  const base = `/api/pbs/tasks/${encodeURIComponent(node)}/${encodeURIComponent(upid)}`;

  const load = useCallback(async () => {
    if (!node || !upid) return;
    const id = ++seqRef.current;
    setLoading(true);
    setError(null);
    try {
      const [statusRes, logRes] = await Promise.all([
        fetch(`${base}/status${query}`, { cache: "no-store" }),
        fetch(`${base}/log${query}`, { cache: "no-store" }),
      ]);
      const statusPayload = (await statusRes.json().catch(() => null)) as { task?: unknown; error?: string } | null;
      const logPayload = (await logRes.json().catch(() => null)) as { lines?: unknown; error?: string } | null;
      if (!statusRes.ok) throw new Error(statusPayload?.error || "");
      if (!logRes.ok) throw new Error(logPayload?.error || "");
      if (seqRef.current !== id) return;
      setStatus(normalizePBSTaskStatus(statusPayload?.task));
      setLog(normalizePBSTaskLogLines(logPayload?.lines));
    } catch (err) {
      if (seqRef.current !== id) return;
      setError([t("tasks.loadFailed"), pbsErrorDetail(err instanceof Error ? err.message : "")].filter(Boolean).join(" "));
      setStatus(null);
      setLog([]);
    } finally {
      if (seqRef.current === id) setLoading(false);
    }
  }, [base, node, query, t, upid]);

  useEffect(() => {
    void load();
  }, [load]);

  const stop = async () => {
    setStopping(true);
    setError(null);
    setStopped(false);
    try {
      const response = await fetch(`${base}/stop${query}`, { method: "POST" });
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      if (!response.ok) {
        setError([t("tasks.stopFailed"), pbsErrorDetail(payload?.error)].filter(Boolean).join(" "));
        return;
      }
      await load();
      setStopped(true);
      onStopped();
    } catch (err) {
      setError([t("tasks.stopFailed"), pbsErrorDetail(err instanceof Error ? err.message : "")].filter(Boolean).join(" "));
    } finally {
      setStopping(false);
    }
  };

  const finished = taskFinished(task, status?.status, status?.exitstatus);
  const name = kindLabel(task.worker_type);

  return (
    <div className="di-pbs-panel" role="region" aria-label={t("tasks.panel", { name })}>
      {error ? <DiNote tone="bad">{error}</DiNote> : null}
      {stopped ? <DiNote tone="ok">{t("tasks.stopRequested", { name })}</DiNote> : null}
      <DiKV
        columns={2}
        items={[
          [t("kv.node"), node],
          [t("kv.state"), status?.status || task.status || t("state.running")],
          [t("kv.exit"), status?.exitstatus || ""],
          [t("kv.target"), task.worker_id || ""],
          [t("kv.user"), task.user || status?.user || ""],
          [t("kv.ended"), task.endtime ? when(task.endtime) : ""],
        ]}
      />
      {log.length > 0 ? (
        <DiLog label={t("tasks.log")}>{log.map((line) => `${String(line.n).padStart(4, " ")}  ${line.t}`).join("\n")}</DiLog>
      ) : (
        <p className="di-pbs-line quiet">{loading ? tk("loadingLog") : tk("noLog")}</p>
      )}
      <div className="di-acts di-pbs-panel-acts">
        <Button size="sm" variant="ghost" onClick={() => void load()} disabled={loading || stopping}>{tk("refresh")}</Button>
        {!finished ? (
          <Button
            size="sm"
            variant="danger"
            disabled={loading || stopping}
            onClick={() => ask({
              title: t("tasks.ask.title", { name }),
              message: t("tasks.ask.body"),
              confirmLabel: t("tasks.stop"),
              onConfirm: stop,
            })}
          >
            {t("tasks.stop")}
          </Button>
        ) : null}
      </div>
      {dialog}
    </div>
  );
}
