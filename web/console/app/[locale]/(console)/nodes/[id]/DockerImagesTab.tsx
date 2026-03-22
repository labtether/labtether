"use client";

import { useEffect, useState } from "react";
import { Card } from "../../../../components/ui/Card";
import type { DockerImage } from "../../../../../lib/docker";
import { fetchDockerImages } from "../../../../../lib/docker";
import { formatBytes } from "../../../../console/formatters";

type Props = { hostId: string };

export function DockerImagesTab({ hostId }: Props) {
  const [images, setImages] = useState<DockerImage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const data = await fetchDockerImages(hostId);
        if (!cancelled) {
          setImages(data);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setImages([]);
          setError(err instanceof Error ? err.message : "failed to load images");
        }
      }
      finally { if (!cancelled) setLoading(false); }
    })();
    return () => { cancelled = true; };
  }, [hostId]);

  return (
    <Card className="mb-4">
      <h2 className="text-sm font-medium text-[var(--text)] mb-3">Images ({images.length})</h2>
      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading images...</p>
      ) : error ? (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--bad)]">Failed to load images</p>
          <p className="text-xs text-[var(--muted)]">{error}</p>
        </div>
      ) : images.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="text-left py-2 text-[var(--muted)] font-medium">Repository:Tag</th>
                <th className="text-left py-2 text-[var(--muted)] font-medium">ID</th>
                <th className="text-right py-2 text-[var(--muted)] font-medium">Size</th>
                <th className="text-left py-2 text-[var(--muted)] font-medium">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {images.map((img) => (
                <tr key={img.id} className="hover:bg-[var(--hover)]">
                  <td className="py-2 text-[var(--text)] font-medium">
                    {img.tags?.length > 0 ? img.tags.join(", ") : "<none>"}
                  </td>
                  <td className="py-2 text-[var(--muted)] font-mono">
                    {img.id?.length > 19 ? img.id.slice(7, 19) : img.id}
                  </td>
                  <td className="py-2 text-right text-[var(--muted)]">{formatBytes(img.size)}</td>
                  <td className="py-2 text-[var(--muted)]">
                    {img.created ? new Date(img.created).toLocaleDateString() : "--"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-12 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No images</p>
          <p className="text-xs text-[var(--muted)]">No images found on this Docker host.</p>
        </div>
      )}
    </Card>
  );
}
