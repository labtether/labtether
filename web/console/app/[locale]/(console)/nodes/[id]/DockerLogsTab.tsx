"use client";

import { useCallback, useEffect, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { fetchDockerContainerLogs } from "../../../../../lib/docker";

type Props = { containerId: string };

export function DockerLogsTab({ containerId }: Props) {
  const [logs, setLogs] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [tail, setTail] = useState(200);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await fetchDockerContainerLogs(containerId, { tail });
      setLogs(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load logs");
    }
    finally { setLoading(false); }
  }, [containerId, tail]);

  useEffect(() => {
    void load();
    if (!autoRefresh) return;
    const interval = setInterval(() => void load(), 5_000);
    return () => clearInterval(interval);
  }, [load, autoRefresh]);

  return (
    <Card className="mb-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Container Logs</h2>
        <div className="flex items-center gap-2">
          <div className="flex gap-1">
            {([100, 200, 500, 1000] as const).map((n) => (
              <button
                key={n}
                className={`px-2 py-1 text-[10px] rounded-lg transition-colors ${
                  tail === n
                    ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                    : "text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]"
                }`}
                onClick={() => setTail(n)}
              >
                {n}
              </button>
            ))}
          </div>
          <button
            className={`px-2 py-1 text-[10px] rounded-lg transition-colors ${
              autoRefresh
                ? "bg-[var(--ok-glow)] text-[var(--ok)]"
                : "text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]"
            }`}
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            Follow
          </button>
          <Button size="sm" onClick={() => void load()}>Refresh</Button>
        </div>
      </div>
      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading logs...</p>
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--bad)]">Failed to load logs</p>
          <p className="text-xs text-[var(--muted)]">{error}</p>
        </div>
      ) : logs ? (
        <pre className="text-xs text-[var(--text)] bg-[var(--surface)] rounded p-3 max-h-[600px] overflow-auto whitespace-pre-wrap font-mono leading-relaxed">
          {logs}
        </pre>
      ) : (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No logs</p>
          <p className="text-xs text-[var(--muted)]">No log output from this container.</p>
        </div>
      )}
    </Card>
  );
}
