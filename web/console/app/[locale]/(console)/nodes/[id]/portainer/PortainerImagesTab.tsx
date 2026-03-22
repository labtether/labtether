"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { Modal } from "../../../../../components/ui/Modal";
import { portainerAction, portainerFetch } from "./usePortainerData";

type DockerImage = {
  Id: string;
  RepoTags?: string[];
  Size: number;
};

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function shortId(id: string): string {
  const stripped = id.startsWith("sha256:") ? id.slice(7) : id;
  return stripped.slice(0, 12);
}

type Props = {
  assetId: string;
};

export function PortainerImagesTab({ assetId }: Props) {
  const [images, setImages] = useState<DockerImage[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [pullOpen, setPullOpen] = useState(false);
  const [pullImage, setPullImage] = useState("");
  const [pulling, setPulling] = useState(false);
  const [pullError, setPullError] = useState<string | null>(null);
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await portainerFetch<DockerImage[]>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/images`,
      );
      if (latestRef.current !== id) return;
      setImages(data ?? []);
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load images");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void load();
  }, [load]);

  const doRemove = useCallback(async (imageId: string) => {
    setActionError(null);
    setActionInFlight(imageId);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/images/${encodeURIComponent(imageId)}`,
        "DELETE",
      );
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "failed to remove image");
    } finally {
      setActionInFlight(null);
    }
  }, [assetId, load]);

  const doPull = useCallback(async () => {
    if (!pullImage.trim()) return;
    setPullError(null);
    setPulling(true);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/images/pull`,
        "POST",
        { image: pullImage.trim() },
      );
      setPullOpen(false);
      setPullImage("");
      await load();
    } catch (err) {
      setPullError(err instanceof Error ? err.message : "failed to pull image");
    } finally {
      setPulling(false);
    }
  }, [assetId, load, pullImage]);

  if (loading && images.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading images…</p></Card>;
  }

  if (error && images.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <>
      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">Images</h2>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={() => { setPullError(null); setPullImage(""); setPullOpen(true); }}>
              Pull Image
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { void load(); }} disabled={loading}>
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
          </div>
        </div>
        {actionError && (
          <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p>
        )}
        {images.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No images found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Tags</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">ID</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Size</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {images.map((img) => (
                  <tr key={img.Id}>
                    <td className="py-2 text-[var(--text)]">
                      {(img.RepoTags ?? []).length > 0
                        ? img.RepoTags!.join(", ")
                        : <span className="text-[var(--muted)]">&lt;none&gt;</span>
                      }
                    </td>
                    <td className="py-2 font-mono text-[var(--muted)]">{shortId(img.Id)}</td>
                    <td className="py-2 text-[var(--muted)]">{formatBytes(img.Size)}</td>
                    <td className="py-2">
                      <div className="flex justify-end">
                        <Button
                          size="sm"
                          variant="danger"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === img.Id}
                          onClick={() => { void doRemove(img.Id); }}
                        >
                          Remove
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal open={pullOpen} onClose={() => setPullOpen(false)} title="Pull Image">
        <div className="p-5 space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1.5">Image Name</label>
            <input
              type="text"
              className="w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
              placeholder="e.g. nginx:latest"
              value={pullImage}
              onChange={(e) => setPullImage(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") { void doPull(); } }}
              autoFocus
            />
          </div>
          {pullError && <p className="text-xs text-[var(--bad)]">{pullError}</p>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setPullOpen(false)} disabled={pulling}>
              Cancel
            </Button>
            <Button variant="primary" size="sm" onClick={() => { void doPull(); }} loading={pulling} disabled={!pullImage.trim()}>
              Pull
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
