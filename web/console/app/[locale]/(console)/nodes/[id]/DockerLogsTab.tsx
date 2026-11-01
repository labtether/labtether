"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePollingTask } from "../../../../hooks/usePollingTask";
import { Button } from "../../../../components/ui/Button";
import { Seg } from "../../../../components/containers/ContainersToolbar";
import { fetchDockerContainerLogs } from "../../../../../lib/docker";

/** The tab's words. The device page passes them translated; the English defaults serve tests without a locale. */
export type DockerLogsWords = {
  lines: string;
  follow: string;
  refresh: string;
  loading: string;
  failed: string;
  none: string;
  label: string;
};
const ENGLISH: DockerLogsWords = {
  lines: "Lines",
  follow: "Follow",
  refresh: "Refresh",
  loading: "Loading logs",
  failed: "Could not load logs",
  none: "No log lines",
  label: "Container logs",
};
const TAILS = ["100", "200", "500", "1000"] as const;

/** A container's last lines, read once (Refresh) or every 5 s while Follow is on. */
export function DockerLogsTab({ containerId, words = ENGLISH }: { containerId: string; words?: DockerLogsWords }) {
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(true);
  const [tail, setTail] = useState<(typeof TAILS)[number]>("200");
  const [follow, setFollow] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const out = useRef<HTMLPreElement>(null);

  const loadTask = useCallback(async (signal: AbortSignal) => {
    try {
      const data = await fetchDockerContainerLogs(containerId, { tail: Number(tail) }, signal);
      if (signal.aborted) return;
      setLogs(data);
      setError(null);
    } catch (err) {
      if (signal.aborted) return;
      setError(err instanceof Error && err.message ? err.message : words.failed);
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, [containerId, tail, words.failed]);

  const load = usePollingTask(loadTask, follow ? 5_000 : 0);

  // While following, keep the newest line in view.
  useEffect(() => {
    if (follow && out.current) out.current.scrollTop = out.current.scrollHeight;
  }, [follow, logs]);

  return (
    <div className="ct-tab">
      <div className="ct-bar">
        <span className="ct-bar-k">{words.lines}</span>
        <Seg value={tail} items={TAILS.map((n) => [n, n])} onChange={setTail} label={words.lines} small />
        <span className="lt-sp" />
        <button type="button" className={`ct-chip${follow ? " on" : ""}`} aria-pressed={follow} onClick={() => setFollow((v) => !v)}>
          {follow ? <i className="ct-live" aria-hidden="true" /> : null}
          {words.follow}
        </button>
        <Button onClick={() => void load()}>{words.refresh}</Button>
      </div>
      {loading && !logs ? (
        <div className="ct-logs ct-quiet" aria-busy="true">{words.loading}</div>
      ) : error && !logs ? (
        <div className="ct-logs ct-quiet" role="alert">{words.failed}: {error}</div>
      ) : logs ? (
        <pre ref={out} className="ct-logs" role="log" aria-label={words.label}>{logs}</pre>
      ) : (
        <div className="ct-logs ct-quiet">{words.none}</div>
      )}
    </div>
  );
}
