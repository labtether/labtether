"use client";
import { useCallback, useState } from "react";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

export type NetworkInterfaceInfo = {
  name: string;
  state: string;
  mac: string;
  mtu: number;
  ips: string[];
  rx_bytes: number;
  tx_bytes: number;
  rx_packets: number;
  tx_packets: number;
};

export function useNetworkInterfaces(assetId: string) {
  const [interfaces, setInterfaces] = useState<NetworkInterfaceInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/network/${encodeURIComponent(assetId)}`, { cache: "no-store" });
      const data = ensureRecord(await res.json().catch(() => null));
      if (!res.ok) {
        throw new Error(ensureString(data?.error) || `Failed (${res.status})`);
      }
      setInterfaces(ensureArray<NetworkInterfaceInfo>(data?.interfaces));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch network interfaces");
    } finally {
      setLoading(false);
    }
  }, [assetId]);

  return { interfaces, loading, error, refresh };
}
