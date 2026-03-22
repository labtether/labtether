"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import type { DockerContainer, DockerStack } from "../../../../../lib/docker";
import { fetchDockerContainers, fetchDockerStacks } from "../../../../../lib/docker";

type Props = {
  hostId: string;
  stackName: string;
};

export function DockerStackInspectTab({ hostId, stackName }: Props) {
  const [stack, setStack] = useState<DockerStack | null>(null);
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [stacks, hostContainers] = await Promise.all([
        fetchDockerStacks(hostId),
        fetchDockerContainers(hostId),
      ]);
      const matchedStack = stacks.find((entry) => entry.name.toLowerCase() === stackName.toLowerCase()) ?? null;
      setStack(matchedStack);
      setContainers(hostContainers);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to inspect stack");
    } finally {
      setLoading(false);
    }
  }, [hostId, stackName]);

  useEffect(() => {
    void load();
  }, [load]);

  const payload = useMemo(() => {
    if (!stack) return null;
    const linked = new Set(stack.container_ids.map((value) => value.toLowerCase()));
    const matchedContainers = containers.filter((container) => {
      if ((container.stack_name ?? "").toLowerCase() === stack.name.toLowerCase()) return true;
      if (linked.has(container.name.toLowerCase())) return true;
      if (linked.has(container.id.toLowerCase())) return true;
      const shortID = container.id.length > 12 ? container.id.slice(0, 12) : container.id;
      return linked.has(shortID.toLowerCase());
    });

    return {
      stack,
      host_id: hostId,
      matched_containers: matchedContainers,
    };
  }, [containers, hostId, stack]);

  return (
    <Card className="mb-4">
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Stack Inspect</h2>
      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading inspect payload...</p>
      ) : payload ? (
        <pre className="max-h-[680px] overflow-auto whitespace-pre-wrap rounded bg-[var(--surface)] p-3 font-mono text-xs text-[var(--text)]">
          {JSON.stringify(payload, null, 2)}
        </pre>
      ) : (
        <div className="flex flex-col items-center justify-center gap-2 py-12">
          <p className="text-sm font-medium text-[var(--text)]">Stack not found</p>
          <p className="text-xs text-[var(--muted)]">This stack may have been removed or renamed.</p>
        </div>
      )}
      {error ? <p className="mt-3 text-xs text-[var(--bad)]">{error}</p> : null}
    </Card>
  );
}

