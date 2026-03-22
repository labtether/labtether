"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type PackageInfo = {
  name: string;
  version: string;
  status: string;
};

export type PackageActionResult = {
  ok: boolean;
  output: string;
  reboot_required?: boolean;
};

export function usePackages(assetId: string) {
  const [packages, setPackages] = useState<PackageInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(
        `/api/packages/${encodeURIComponent(assetId)}`,
        { cache: "no-store" }
      );
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      setPackages(ensureArray<PackageInfo>(data?.packages));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch packages");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  const performAction = useCallback(async (action: "install" | "remove" | "upgrade", packageNames: string[]) => {
    const normalized = Array.from(new Set(packageNames.map((name) => name.trim()).filter((name) => name.length > 0)));
    const res = await fetch(`/api/packages/${encodeURIComponent(assetId)}/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ packages: normalized }),
      cache: "no-store",
    });
    const data = ensureRecord(await res.json().catch(() => null)) as Partial<PackageActionResult> & { error?: unknown };
    if (!res.ok) {
      throw new Error(ensureString(data.error) || `Failed (${res.status})`);
    }
    await refresh();
    return {
      ok: data.ok === true,
      output: ensureString(data.output),
      reboot_required: data.reboot_required === true ? true : undefined,
    };
  }, [assetId, refresh]);

  return { packages, loading, error, refresh, performAction };
}
