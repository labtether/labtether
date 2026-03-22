"use client";

import { useState, useEffect, useCallback } from "react";
import type { EnrollmentToken, AgentTokenSummary } from "../console/models";
import { ensureArray, ensureRecord, ensureString } from "../lib/responseGuards";

const HUB_SELECTION_STORAGE_KEY = "labtether.enrollment.selectedHubURL";

export type HubConnectionCandidate = {
  kind: string;
  label: string;
  host: string;
  hub_url: string;
  ws_url: string;
  preferred_reason?: string;
  trust_mode?: string;
  bootstrap_url?: string;
  bootstrap_strategy?: string;
};

type EnrollmentState = {
  enrollmentTokens: EnrollmentToken[];
  agentTokens: AgentTokenSummary[];
  hubCandidates: HubConnectionCandidate[];
  hubURL: string;
  wsURL: string;
  selectHubURL: (hubURL: string) => void;
  loading: boolean;
  error: string;
  newRawToken: string;
  newTokenID: string;
  generating: boolean;
  generateToken: (label: string, ttlHours: number, maxUses: number) => Promise<void>;
  revokeEnrollmentToken: (id: string) => Promise<void>;
  revokeAgentToken: (id: string) => Promise<void>;
  cleanupDeadTokens: () => Promise<{ enrollment_deleted: number; agent_deleted: number } | null>;
  clearNewToken: () => void;
  refresh: () => void;
};

function readStoredHubSelection(): string {
  if (typeof window === "undefined") {
    return "";
  }
  try {
    return window.localStorage.getItem(HUB_SELECTION_STORAGE_KEY)?.trim() ?? "";
  } catch {
    return "";
  }
}

export function useEnrollment(): EnrollmentState {
  const [enrollmentTokens, setEnrollmentTokens] = useState<EnrollmentToken[]>([]);
  const [agentTokens, setAgentTokens] = useState<AgentTokenSummary[]>([]);
  const [hubCandidates, setHubCandidates] = useState<HubConnectionCandidate[]>([]);
  const [defaultHubURL, setDefaultHubURL] = useState("");
  const [defaultWsURL, setDefaultWsURL] = useState("");
  const [selectedHubURL, setSelectedHubURL] = useState(() => readStoredHubSelection());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [newRawToken, setNewRawToken] = useState("");
  const [newTokenID, setNewTokenID] = useState("");
  const [generating, setGenerating] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    try {
      const normalized = selectedHubURL.trim();
      if (normalized) {
        window.localStorage.setItem(HUB_SELECTION_STORAGE_KEY, normalized);
      } else {
        window.localStorage.removeItem(HUB_SELECTION_STORAGE_KEY);
      }
    } catch {
      // Ignore local preference persistence failures and keep setup usable.
    }
  }, [selectedHubURL]);

  const normalizeCandidates = useCallback((rawCandidates: unknown, fallbackHubURL: string, fallbackWsURL: string) => {
    const normalized: HubConnectionCandidate[] = [];
    const seen = new Set<string>();

    if (Array.isArray(rawCandidates)) {
      for (const item of rawCandidates) {
        if (!item || typeof item !== "object") continue;
        const candidate = item as Partial<HubConnectionCandidate>;
        const hubURL = typeof candidate.hub_url === "string" ? candidate.hub_url.trim() : "";
        const wsURL = typeof candidate.ws_url === "string" ? candidate.ws_url.trim() : "";
        const host = typeof candidate.host === "string" ? candidate.host.trim() : "";
        if (!hubURL || !wsURL || seen.has(hubURL)) continue;
        seen.add(hubURL);
        normalized.push({
          kind: typeof candidate.kind === "string" ? candidate.kind.trim() : "",
          label: typeof candidate.label === "string" ? candidate.label.trim() : "",
          host,
          hub_url: hubURL,
          ws_url: wsURL,
          preferred_reason: typeof candidate.preferred_reason === "string" ? candidate.preferred_reason.trim() : "",
          trust_mode: typeof candidate.trust_mode === "string" ? candidate.trust_mode.trim() : "",
          bootstrap_url: typeof candidate.bootstrap_url === "string" ? candidate.bootstrap_url.trim() : "",
          bootstrap_strategy: typeof candidate.bootstrap_strategy === "string" ? candidate.bootstrap_strategy.trim() : "",
        });
      }
    }

    if (fallbackHubURL && fallbackWsURL && !seen.has(fallbackHubURL)) {
      normalized.push({
        kind: "default",
        label: "Default",
        host: "",
        hub_url: fallbackHubURL,
        ws_url: fallbackWsURL,
        preferred_reason: "",
        trust_mode: "",
        bootstrap_url: "",
        bootstrap_strategy: "",
      });
    }

    return normalized;
  }, []);

  const fetchEnrollmentTokens = useCallback(async () => {
    try {
      const res = await fetch("/api/settings/enrollment");
      if (!res.ok) throw new Error("Failed to fetch enrollment tokens");
      const data = ensureRecord(await res.json().catch(() => null));
      const fetchedHubURL = ensureString(data?.hub_url).trim();
      const fetchedWsURL = ensureString(data?.ws_url).trim();
      const candidates = normalizeCandidates(data?.hub_candidates, fetchedHubURL, fetchedWsURL);
      setEnrollmentTokens(ensureArray<EnrollmentToken>(data?.tokens));
      setDefaultHubURL(fetchedHubURL);
      setDefaultWsURL(fetchedWsURL);
      setHubCandidates(candidates);
      setSelectedHubURL((current) => {
        if (current && candidates.some((candidate) => candidate.hub_url === current)) {
          return current;
        }
        if (fetchedHubURL && candidates.some((candidate) => candidate.hub_url === fetchedHubURL)) {
          return fetchedHubURL;
        }
        return candidates[0]?.hub_url ?? fetchedHubURL;
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    }
  }, [normalizeCandidates]);

  const fetchAgentTokens = useCallback(async () => {
    try {
      const res = await fetch("/api/settings/agent-tokens");
      if (!res.ok) throw new Error("Failed to fetch agent tokens");
      const data = ensureRecord(await res.json().catch(() => null));
      setAgentTokens(ensureArray<AgentTokenSummary>(data?.tokens));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    }
  }, []);

  const refresh = useCallback(() => {
    setLoading(true);
    setError("");
    Promise.all([fetchEnrollmentTokens(), fetchAgentTokens()]).finally(() => setLoading(false));
  }, [fetchEnrollmentTokens, fetchAgentTokens]);

  useEffect(() => { refresh(); }, [refresh]);

  const generateToken = useCallback(async (label: string, ttlHours: number, maxUses: number) => {
    setGenerating(true);
    setError("");
    setNewRawToken("");
    setNewTokenID("");
    try {
      const normalizedMaxUses = Number.isFinite(maxUses) && maxUses > 0 ? Math.floor(maxUses) : 1;
      const res = await fetch("/api/settings/enrollment", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ label, ttl_hours: ttlHours, max_uses: normalizedMaxUses }),
      });
      if (!res.ok) throw new Error("Failed to generate enrollment token");
      const data = ensureRecord(await res.json().catch(() => null));
      setNewRawToken(ensureString(data?.raw_token));
      setNewTokenID(ensureString(ensureRecord(data?.token)?.id));
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setGenerating(false);
    }
  }, [refresh]);

  const revokeEnrollmentToken = useCallback(async (id: string) => {
    try {
      const res = await fetch(`/api/settings/enrollment/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (!res.ok) throw new Error("Failed to revoke token");
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    }
  }, [refresh]);

  const revokeAgentToken = useCallback(async (id: string) => {
    try {
      const res = await fetch(`/api/settings/agent-tokens/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (!res.ok) throw new Error("Failed to revoke agent token");
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    }
  }, [refresh]);

  const cleanupDeadTokens = useCallback(async (): Promise<{ enrollment_deleted: number; agent_deleted: number } | null> => {
    try {
      const res = await fetch("/api/settings/tokens/cleanup", { method: "DELETE" });
      if (!res.ok) throw new Error("Failed to clean up tokens");
      const data = ensureRecord(await res.json().catch(() => null));
      refresh();
      return {
        enrollment_deleted: typeof data?.enrollment_deleted === "number" ? data.enrollment_deleted : 0,
        agent_deleted: typeof data?.agent_deleted === "number" ? data.agent_deleted : 0,
      };
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
      return null;
    }
  }, [refresh]);

  const clearNewToken = useCallback(() => {
    setNewRawToken("");
    setNewTokenID("");
  }, []);
  const selectedCandidate = hubCandidates.find((candidate) => candidate.hub_url === selectedHubURL);
  const hubURL = selectedCandidate?.hub_url ?? defaultHubURL;
  const wsURL = selectedCandidate?.ws_url ?? defaultWsURL;
  const selectHubURL = useCallback((hubURL: string) => {
    setSelectedHubURL(hubURL.trim());
  }, []);

  return {
    enrollmentTokens, agentTokens, hubCandidates, hubURL, wsURL, selectHubURL,
    loading, error, newRawToken, newTokenID, generating,
    generateToken, revokeEnrollmentToken, revokeAgentToken, cleanupDeadTokens, clearNewToken, refresh,
  };
}
