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

export default function NewContainerPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();

  const routeNodeID = decodeURIComponent((params.id ?? "").trim());
  const normalizedHostID = useMemo(() => normalizeHostID(routeNodeID), [routeNodeID]);
  const hostNodeID = normalizedHostID ? `docker-host-${normalizedHostID}` : routeNodeID;
  const hostActionTarget = normalizedHostID || routeNodeID;

  const [image, setImage] = useState("");
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [environment, setEnvironment] = useState("");
  const [ports, setPorts] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdID, setCreatedID] = useState<string>("");

  const hostContainersHref = `/nodes/${encodeURIComponent(hostNodeID)}?panel=docker&sub=containers`;
  const hostStacksHref = `/nodes/${encodeURIComponent(hostNodeID)}?panel=docker&sub=stacks`;

  const createdContainerAssetID = useMemo(() => {
    const trimmed = createdID.trim();
    if (!trimmed || !normalizedHostID) return "";
    const shortID = trimmed.length > 12 ? trimmed.slice(0, 12) : trimmed;
    return `docker-ct-${normalizedHostID}-${shortID}`;
  }, [createdID, normalizedHostID]);

  async function onCreate() {
    const trimmedImage = image.trim();
    if (!trimmedImage) {
      setError("Image is required.");
      return;
    }

    setSubmitting(true);
    setError(null);
    setCreatedID("");
    try {
      const result = await executeDockerHostAction(hostActionTarget, "container.create", {
        image: trimmedImage,
        name: name.trim(),
        command: command.trim(),
        env: environment.trim(),
        ports: ports.trim(),
      });
      if (result.status !== "succeeded") {
        throw new Error(result.message || "container creation failed");
      }
      const newID = (result.output ?? "").trim();
      setCreatedID(newID);
      setName("");
      setCommand("");
      setEnvironment("");
      setPorts("");
      setTimeout(() => {
        router.push(hostContainersHref);
      }, 900);
    } catch (err) {
      setError(err instanceof Error ? err.message : "container creation failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <>
      <PageHeader
        title="New Container"
        subtitle={(
          <span className="flex flex-wrap items-center gap-1">
            <Link href="/nodes" className="text-[var(--accent)] hover:underline">Devices</Link>
            <span>/</span>
            <Link href={`/nodes/${encodeURIComponent(hostNodeID)}`} className="text-[var(--accent)] hover:underline">
              {hostNodeID}
            </Link>
            <span>/</span>
            <span>Create Container</span>
          </span>
        )}
      />

      <Card className="mb-4">
        <div className="mb-4 rounded-xl border border-[var(--line)] bg-[linear-gradient(135deg,var(--panel)_0%%,color-mix(in_oklab,var(--accent)_10%%,var(--panel))_100%%)] p-4">
          <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">Container Builder</p>
          <p className="mt-1 text-sm text-[var(--text)]">
            Launch a new container on <span className="font-mono">{hostActionTarget}</span>. Use one value per line for
            environment variables and one mapping per line for ports.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <label className="space-y-1">
            <span className="text-xs font-medium text-[var(--muted)]">Image *</span>
            <input
              value={image}
              onChange={(event) => setImage(event.target.value)}
              placeholder="e.g. nginx:latest"
              className="h-10 w-full rounded-md border border-[var(--line)] bg-transparent px-3 text-sm text-[var(--text)]"
            />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-medium text-[var(--muted)]">Container Name</span>
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="optional"
              className="h-10 w-full rounded-md border border-[var(--line)] bg-transparent px-3 text-sm text-[var(--text)]"
            />
          </label>
          <label className="space-y-1 md:col-span-2">
            <span className="text-xs font-medium text-[var(--muted)]">Command Override</span>
            <input
              value={command}
              onChange={(event) => setCommand(event.target.value)}
              placeholder="optional, e.g. python -m http.server 8080"
              className="h-10 w-full rounded-md border border-[var(--line)] bg-transparent px-3 text-sm text-[var(--text)]"
            />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-medium text-[var(--muted)]">Environment Variables</span>
            <textarea
              value={environment}
              onChange={(event) => setEnvironment(event.target.value)}
              placeholder={"TZ=America/New_York\nPUID=1000\nPGID=1000"}
              className="min-h-36 w-full rounded-md border border-[var(--line)] bg-transparent px-3 py-2 text-xs text-[var(--text)] font-mono"
            />
          </label>
          <label className="space-y-1">
            <span className="text-xs font-medium text-[var(--muted)]">Port Mappings</span>
            <textarea
              value={ports}
              onChange={(event) => setPorts(event.target.value)}
              placeholder={"8080:80\n9443:9443/tcp\n5353:5353/udp"}
              className="min-h-36 w-full rounded-md border border-[var(--line)] bg-transparent px-3 py-2 text-xs text-[var(--text)] font-mono"
            />
          </label>
        </div>

        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Button size="md" disabled={submitting} onClick={() => void onCreate()}>
            {submitting ? "Creating..." : "Create Container"}
          </Button>
          <Link
            href={hostContainersHref}
            className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
          >
            Back to Containers
          </Link>
          <Link
            href={hostStacksHref}
            className="inline-flex items-center justify-center rounded-lg border border-[var(--control-border)] px-3 py-1.5 text-xs font-medium text-[var(--control-fg)] transition-colors hover:bg-[var(--control-bg-hover)]"
          >
            Go to Stacks
          </Link>
        </div>

        {error ? <p className="mt-3 text-xs text-[var(--bad)]">{error}</p> : null}
        {!error && createdID ? (
          <div className="mt-3 space-y-1">
            <p className="text-xs text-[var(--ok)]">Container created: <span className="font-mono">{createdID}</span></p>
            {createdContainerAssetID ? (
              <Link
                href={`/nodes/${encodeURIComponent(createdContainerAssetID)}`}
                className="text-xs text-[var(--accent)] hover:underline"
              >
                Open container details
              </Link>
            ) : null}
          </div>
        ) : null}
      </Card>
    </>
  );
}

