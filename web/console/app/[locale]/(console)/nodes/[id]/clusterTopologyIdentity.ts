import type { Asset } from "../../../../console/models";

export type IdentitySnapshot = {
  ips: string[];
  macs: string[];
  uuids: string[];
  hostnames: string[];
};

type IdentityMatch = {
  hostID: string;
  reason: "ip" | "mac" | "uuid" | "hostname";
  score: number;
};

type IdentityAccumulator = {
  ips: Set<string>;
  macs: Set<string>;
  uuids: Set<string>;
  hostnames: Set<string>;
};

export function normalizeAssetName(value?: string): string {
  return (value ?? "").trim().toLowerCase().replace(/[^a-z0-9]/g, "");
}

export function pickStrongIdentityMatch(
  guest: IdentitySnapshot,
  hostsByID: Map<string, IdentitySnapshot>,
  hostAssetsByID: Map<string, Asset>,
): { hostID: string; reason: "ip" | "mac" | "uuid" | "hostname"; score: number } | null {
  const guestIPs = new Set(guest.ips);
  const guestMACs = new Set(guest.macs);
  const guestUUIDs = new Set(guest.uuids);
  const guestHostnames = new Set(guest.hostnames);

  let bestScore = -1;
  let bestMatches: IdentityMatch[] = [];

  for (const [hostID, hostIdentity] of hostsByID.entries()) {
    const uuidMatches = overlapCount(hostIdentity.uuids, guestUUIDs);
    const macMatches = overlapCount(hostIdentity.macs, guestMACs);
    const ipMatches = overlapCount(hostIdentity.ips, guestIPs);
    if (uuidMatches === 0 && macMatches === 0 && ipMatches === 0) {
      continue;
    }

    const score = (uuidMatches * 100) + (macMatches * 60) + (ipMatches * 40);
    const reason: "ip" | "mac" | "uuid" = uuidMatches > 0 ? "uuid" : (macMatches > 0 ? "mac" : "ip");
    const current: IdentityMatch = { hostID, reason, score };

    if (current.score > bestScore) {
      bestScore = current.score;
      bestMatches = [current];
      continue;
    }
    if (current.score === bestScore) {
      bestMatches.push(current);
    }
  }

  if (bestMatches.length === 1) {
    return bestMatches[0] ?? null;
  }
  if (bestMatches.length > 1) {
    const resolved = resolveIdentityTie(bestMatches, hostAssetsByID);
    if (resolved) {
      return resolved;
    }
    return null;
  }

  if (guestHostnames.size === 0) {
    return null;
  }
  let hostnameBestScore = -1;
  let hostnameBestMatches: IdentityMatch[] = [];
  for (const [hostID, hostIdentity] of hostsByID.entries()) {
    const hostnameMatches = overlapCount(hostIdentity.hostnames, guestHostnames);
    if (hostnameMatches === 0) {
      continue;
    }
    const candidate: IdentityMatch = {
      hostID,
      reason: "hostname",
      score: hostnameMatches,
    };
    if (candidate.score > hostnameBestScore) {
      hostnameBestScore = candidate.score;
      hostnameBestMatches = [candidate];
      continue;
    }
    if (candidate.score === hostnameBestScore) {
      hostnameBestMatches.push(candidate);
    }
  }
  if (hostnameBestMatches.length === 1) {
    return hostnameBestMatches[0] ?? null;
  }
  if (hostnameBestMatches.length > 1) {
    return resolveIdentityTie(hostnameBestMatches, hostAssetsByID);
  }
  return null;
}

export function collectIdentityFromAsset(asset: Asset): IdentitySnapshot {
  const acc = newIdentityAccumulator();
  addHostToken(acc, asset.name);
  const metadata = asset.metadata ?? {};
  for (const [key, value] of Object.entries(metadata)) {
    collectIdentityFromValue(acc, key, value);
  }
  return finalizeIdentity(acc);
}

export function collectIdentityFromRecord(record: Record<string, unknown>): IdentitySnapshot {
  const acc = newIdentityAccumulator();
  for (const [key, value] of Object.entries(record)) {
    if (typeof value === "string") {
      collectIdentityFromValue(acc, key, value);
      continue;
    }
    if (typeof value === "number" || typeof value === "boolean") {
      collectIdentityFromValue(acc, key, String(value));
    }
  }
  return finalizeIdentity(acc);
}

export function mergeIdentitySnapshots(first: IdentitySnapshot, second: IdentitySnapshot): IdentitySnapshot {
  return {
    ips: uniqueSorted([...first.ips, ...second.ips]),
    macs: uniqueSorted([...first.macs, ...second.macs]),
    uuids: uniqueSorted([...first.uuids, ...second.uuids]),
    hostnames: uniqueSorted([...first.hostnames, ...second.hostnames]),
  };
}

function resolveIdentityTie(matches: IdentityMatch[], hostAssetsByID: Map<string, Asset>): IdentityMatch | null {
  if (matches.length === 0) {
    return null;
  }

  let best = matches[0]!;
  let bestRank = hostIdentityPriority(hostAssetsByID.get(best.hostID));
  let tied = false;

  for (const candidate of matches.slice(1)) {
    const rank = hostIdentityPriority(hostAssetsByID.get(candidate.hostID));
    if (rank < bestRank) {
      best = candidate;
      bestRank = rank;
      tied = false;
      continue;
    }
    if (rank === bestRank) {
      tied = true;
    }
  }

  if (tied) {
    return null;
  }
  return best;
}

function hostIdentityPriority(host?: Asset): number {
  if (!host) {
    return 99;
  }
  const source = (host.source ?? "").trim().toLowerCase();
  if (source === "truenas") {
    return 0;
  }
  if (source === "docker") {
    return 1;
  }
  if (source === "portainer") {
    return 2;
  }
  return 10;
}

function overlapCount(values: string[], candidates: Set<string>): number {
  let count = 0;
  for (const value of values) {
    if (candidates.has(value)) {
      count++;
    }
  }
  return count;
}

function newIdentityAccumulator(): IdentityAccumulator {
  return {
    ips: new Set<string>(),
    macs: new Set<string>(),
    uuids: new Set<string>(),
    hostnames: new Set<string>(),
  };
}

function finalizeIdentity(acc: IdentityAccumulator): IdentitySnapshot {
  return {
    ips: uniqueSorted([...acc.ips]),
    macs: uniqueSorted([...acc.macs]),
    uuids: uniqueSorted([...acc.uuids]),
    hostnames: uniqueSorted([...acc.hostnames]),
  };
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values)].sort((a, b) => a.localeCompare(b));
}

function collectIdentityFromValue(acc: IdentityAccumulator, rawKey: string, rawValue: string): void {
  const key = rawKey.trim().toLowerCase();
  const value = rawValue.trim();
  if (!value) {
    return;
  }

  for (const ip of extractIPv4(value)) {
    if (isUsableIdentityIP(ip)) {
      acc.ips.add(ip);
    }
  }
  for (const mac of extractMAC(value)) {
    acc.macs.add(mac);
  }
  for (const uuid of extractUUID(value)) {
    acc.uuids.add(uuid);
  }

  if (
    key.includes("url")
    || key.includes("endpoint")
    || key.includes("base_url")
    || key.includes("address")
    || key.includes("host")
  ) {
    const urlHost = parseURLHost(value);
    if (urlHost) {
      if (isUsableIdentityIP(urlHost)) {
        acc.ips.add(urlHost);
      }
      addHostToken(acc, urlHost);
    }
  }

  if (
    key.includes("hostname")
    || key === "host"
    || key.endsWith("_host")
    || key.includes("dns")
    || key.includes("name")
  ) {
    addHostToken(acc, value);
  }
}

function addHostToken(acc: IdentityAccumulator, raw: string): void {
  const token = normalizeHost(raw);
  if (!token) {
    return;
  }
  if (token === "localhost") {
    return;
  }
  acc.hostnames.add(token);
}

function normalizeHost(raw: string): string {
  let value = raw.trim().toLowerCase();
  if (!value) {
    return "";
  }
  if (value.includes("://")) {
    const parsed = parseURLHost(value);
    value = parsed ?? value;
  }
  if (value.includes("/")) {
    value = value.split("/")[0] ?? value;
  }
  if (value.includes(":")) {
    const ipv6 = value.startsWith("[") && value.endsWith("]");
    if (!ipv6 && value.split(":").length === 2) {
      value = value.split(":")[0] ?? value;
    }
  }
  value = value.replace(/\.+$/, "");
  if (!/^[a-z0-9.-]+$/.test(value)) {
    return "";
  }
  return value;
}

function parseURLHost(raw: string): string | null {
  const value = raw.trim();
  if (!value) {
    return null;
  }
  try {
    const parsed = new URL(value.includes("://") ? value : `https://${value}`);
    const host = parsed.hostname.trim().toLowerCase();
    if (!host) {
      return null;
    }
    return host;
  } catch {
    return null;
  }
}

function extractIPv4(text: string): string[] {
  const matches = text.match(/\b(?:\d{1,3}\.){3}\d{1,3}\b/g);
  if (!matches) {
    return [];
  }
  const out: string[] = [];
  for (const match of matches) {
    const parts = match.split(".");
    if (parts.length !== 4) {
      continue;
    }
    const valid = parts.every((part) => {
      const parsed = Number.parseInt(part, 10);
      return !Number.isNaN(parsed) && parsed >= 0 && parsed <= 255;
    });
    if (valid) {
      out.push(match);
    }
  }
  return out;
}

function extractMAC(text: string): string[] {
  const matches = text.match(/\b(?:[0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}\b/g);
  if (!matches) {
    return [];
  }
  return matches.map((value) => value.toLowerCase().replace(/-/g, ":"));
}

function extractUUID(text: string): string[] {
  const matches = text.match(/\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b/g);
  if (!matches) {
    return [];
  }
  return matches.map((value) => value.toLowerCase());
}

function isUsableIdentityIP(ip: string): boolean {
  const value = ip.trim();
  if (!value) {
    return false;
  }
  if (value === "0.0.0.0") {
    return false;
  }
  if (value.startsWith("127.")) {
    return false;
  }
  if (value.startsWith("169.254.")) {
    return false;
  }
  return true;
}
