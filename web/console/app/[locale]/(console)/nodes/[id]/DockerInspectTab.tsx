"use client";

import { useEffect, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import type { DockerContainer } from "../../../../../lib/docker";
import { fetchDockerContainerDetail } from "../../../../../lib/docker";

type Props = { containerId: string };

export function DockerInspectTab({ containerId }: Props) {
  const [container, setContainer] = useState<DockerContainer | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const data = await fetchDockerContainerDetail(containerId);
        if (!cancelled) setContainer(data);
        if (!cancelled) setError(null);
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load container details");
        }
      }
      finally { if (!cancelled) setLoading(false); }
    })();
    return () => { cancelled = true; };
  }, [containerId]);

  return (
    <Card className="mb-4">
      <h2 className="text-sm font-medium text-[var(--text)] mb-3">Container Details</h2>
      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading...</p>
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--bad)]">Failed to load container details</p>
          <p className="text-xs text-[var(--muted)]">{error}</p>
        </div>
      ) : container ? (
        <div className="space-y-4">
          <dl className="grid grid-cols-2 gap-x-6 gap-y-2">
            <div>
              <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">Name</dt>
              <dd className="text-xs text-[var(--text)]">{container.name}</dd>
            </div>
            <div>
              <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">Image</dt>
              <dd className="text-xs text-[var(--text)]">{container.image}</dd>
            </div>
            <div>
              <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">State</dt>
              <dd className="text-xs text-[var(--text)]">{container.state}</dd>
            </div>
            <div>
              <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">Status</dt>
              <dd className="text-xs text-[var(--text)]">{container.status}</dd>
            </div>
            <div>
              <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">Container ID</dt>
              <dd className="text-xs text-[var(--text)] font-mono">{container.id}</dd>
            </div>
            <div>
              <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">Created</dt>
              <dd className="text-xs text-[var(--text)]">{container.created ? new Date(container.created).toLocaleString() : "--"}</dd>
            </div>
            {container.ports ? (
              <div className="col-span-2">
                <dt className="text-[10px] text-[var(--muted)] uppercase tracking-wider">Ports</dt>
                <dd className="text-xs text-[var(--text)] font-mono">{container.ports}</dd>
              </div>
            ) : null}
          </dl>

          {container.labels && Object.keys(container.labels).length > 0 ? (
            <div>
              <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">Labels</p>
              <pre className="text-xs text-[var(--text)] bg-[var(--surface)] rounded p-3 max-h-64 overflow-auto whitespace-pre-wrap font-mono">
                {JSON.stringify(container.labels, null, 2)}
              </pre>
            </div>
          ) : null}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">Container not found</p>
        </div>
      )}
    </Card>
  );
}
