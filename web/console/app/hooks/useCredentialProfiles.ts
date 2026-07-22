"use client";

import { useCallback, useEffect, useState } from "react";

export const credentialKinds = [
  "ssh_password",
  "ssh_private_key",
  "vnc_password",
  "proxmox_api_token",
  "proxmox_password",
  "pbs_api_token",
  "portainer_api_key",
  "truenas_api_key",
  "homeassistant_token",
  "telnet_password",
  "rdp_password",
  "ftp_password",
  "smb_credentials",
  "webdav_credentials",
] as const;

export type CredentialKind = (typeof credentialKinds)[number];

export type CredentialProfile = {
  id: string;
  name: string;
  kind: string;
  username?: string;
  description?: string;
  status?: string;
  metadata?: Record<string, string>;
  created_at: string;
  updated_at: string;
  rotated_at?: string;
  last_used_at?: string;
  expires_at?: string;
};

type CredentialProfilesEnvelope = { profiles?: unknown };

function isCredentialProfile(value: unknown): value is CredentialProfile {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const profile = value as Record<string, unknown>;
  return typeof profile.id === "string"
    && profile.id.length > 0
    && typeof profile.name === "string"
    && typeof profile.kind === "string";
}

export async function credentialResponseError(response: Response, fallback: string): Promise<string> {
  try {
    const payload = await response.json() as { error?: unknown };
    if (typeof payload.error === "string" && payload.error.trim()) return payload.error.trim();
  } catch {
    // Use the explicit fallback below.
  }
  return fallback;
}

export function useCredentialProfiles(enabled = true) {
  const [profiles, setProfiles] = useState<CredentialProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const refresh = useCallback(() => setRefreshKey((value) => value + 1), []);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return undefined;
    }
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void (async () => {
      try {
        const response = await fetch("/api/settings/credentials?limit=500", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) {
          setProfiles([]);
          setError(await credentialResponseError(response, `Failed to load credential profiles (${response.status}).`));
          return;
        }
        const payload = await response.json() as CredentialProfilesEnvelope;
        if (!Array.isArray(payload.profiles)) {
          setProfiles([]);
          setError("Credential profile response was invalid.");
          return;
        }
        setProfiles(payload.profiles.filter(isCredentialProfile));
      } catch (loadError) {
        if (controller.signal.aborted) return;
        setProfiles([]);
        setError(loadError instanceof Error ? loadError.message : "Failed to load credential profiles.");
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [enabled, refreshKey]);

  return { profiles, loading, error, refresh };
}
