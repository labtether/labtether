"use client";

import { useMemo, useState } from "react";
import { useParams } from "next/navigation";
import { useRouter, Link } from "../../../../../../i18n/navigation";
import { PageHeader } from "../../../../../components/PageHeader";
import { Card } from "../../../../../components/ui/Card";
import { Button } from "../../../../../components/ui/Button";
import { executeDockerHostAction } from "../../../../../../lib/docker";

function normalizeHostID(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replaceAll(" ", "-")
    .replaceAll(".", "-")
    .replace(/^docker-host-/, "")
    .replace(/^docker-/, "");
}

const composeTemplate = `services:
  app:
    image: nginx:latest
    ports:
      - "8080:80"
    restart: unless-stopped
`;

export default function NewComposePage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();

  const routeNodeID = decodeURIComponent((params.id ?? "").trim());
  const normalizedHostID = useMemo(() => normalizeHostID(routeNodeID), [routeNodeID]);
  const hostNodeID = normalizedHostID ? `docker-host-${normalizedHostID}` : routeNodeID;
  const hostActionTarget = normalizedHostID || routeNodeID;

  const [stackName, setStackName] = useState("");
  const [composeYAML, setComposeYAML] = useState(composeTemplate);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const hostStacksHref = `/nodes/${encodeURIComponent(hostNodeID)}?panel=docker&sub=stacks`;
  const hostContainersHref = `/nodes/${encodeURIComponent(hostNodeID)}?panel=docker&sub=containers`;

  async function onDeploy() {
    const trimmedName = stackName.trim();
    const trimmedYAML = composeYAML.trim();
    if (!trimmedName) {
      setError("Stack name is required.");
      return;
    }
    if (!trimmedYAML) {
      setError("Compose YAML is required.");
      return;
    }

    setSubmitting(true);
    setError(null);
    setSuccessMessage(null);
    try {
      const result = await executeDockerHostAction(hostActionTarget, "stack.deploy", {
        stack_name: trimmedName,
        compose_yaml: trimmedYAML,
      });
      if (result.status !== "succeeded") {
        throw new Error(result.message || "stack deploy failed");
      }
      setSuccessMessage(`Compose stack "${trimmedName}" deployed.`);
      setTimeout(() => {
        router.push(hostStacksHref);
      }, 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "stack deploy failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <>
      <PageHeader
        title="New Compose Stack"
        subtitle={(
          <span className="flex flex-wrap items-center gap-1">
            <Link href="/nodes" className="text-[var(--accent)] hover:underline">Devices</Link>
            <span>/</span>
            <Link href={`/nodes/${encodeURIComponent(hostNodeID)}`} className="text-[var(--accent)] hover:underline">
              {hostNodeID}
            </Link>
            <span>/</span>
            <span>Deploy Compose</span>
          </span>
        )}
      />

      <Card className="mb-4">
        <div className="mb-4 rounded-xl border border-[var(--line)] bg-[linear-gradient(135deg,var(--panel)_0%%,color-mix(in_oklab,var(--accent)_10%%,var(--panel))_100%%)] p-4">
          <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">Compose Deployment</p>
          <p className="mt-1 text-sm text-[var(--text)]">
            Deploy a stack on <span className="font-mono">{hostActionTarget}</span> with a focused editor flow.
          </p>
        </div>

        <div className="space-y-3">
          <label className="space-y-1">
            <span className="text-xs font-medium text-[var(--muted)]">Stack Name *</span>
            <input
              value={stackName}
              onChange={(event) => setStackName(event.target.value)}
              placeholder="e.g. media, monitoring, reverse-proxy"
              className="h-10 w-full rounded-md border border-[var(--line)] bg-transparent px-3 text-sm text-[var(--text)]"
            />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-medium text-[var(--muted)]">Compose YAML *</span>
            <textarea
              value={composeYAML}
              onChange={(event) => setComposeYAML(event.target.value)}
              className="min-h-[340px] w-full rounded-md border border-[var(--line)] bg-transparent px-3 py-2 font-mono text-xs text-[var(--text)]"
            />
          </label>
        </div>

        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Button size="md" disabled={submitting} onClick={() => void onDeploy()}>
            {submitting ? "Deploying..." : "Deploy Stack"}
          </Button>
          <Link
            href={hostStacksHref}
            className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
          >
            Back to Stacks
          </Link>
          <Link
            href={hostContainersHref}
            className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
          >
            Go to Containers
          </Link>
        </div>

        {error ? <p className="mt-3 text-xs text-[var(--bad)]">{error}</p> : null}
        {!error && successMessage ? <p className="mt-3 text-xs text-[var(--ok)]">{successMessage}</p> : null}
      </Card>
    </>
  );
}

