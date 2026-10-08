"use client";

import type { RuntimeSettingEntry } from "../../../../console/models";

// ---------------------------------------------------------------------------
// Shared types & helpers
// ---------------------------------------------------------------------------

export type GroupingMode = "off" | "conservative" | "balanced" | "aggressive";

export function normalizeMode(value: string): GroupingMode {
  const lowered = value.trim().toLowerCase();
  if (
    lowered === "off" ||
    lowered === "conservative" ||
    lowered === "balanced" ||
    lowered === "aggressive"
  ) {
    return lowered;
  }
  return "balanced";
}

export function normalizeThreshold(value: string, fallback: string): string {
  const trimmed = value.trim();
  if (!/^[0-9]+$/.test(trimmed)) return fallback;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed)) return fallback;
  return String(Math.max(0, Math.min(100, parsed)));
}

export function parseBool(value: string, fallback: boolean): boolean {
  const lowered = value.trim().toLowerCase();
  if (lowered === "true") return true;
  if (lowered === "false") return false;
  return fallback;
}

export function appendRuleLine(existing: string, nextRule: string): string {
  const normalizedRule = nextRule.trim();
  if (!normalizedRule) return existing;
  const lines = existing
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
  const seen = new Set(lines.map((line) => line.toLowerCase()));
  if (!seen.has(normalizedRule.toLowerCase())) {
    lines.push(normalizedRule);
  }
  return lines.join("\n");
}

export function normalizeBaseDomain(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^\*?\./, "")
    .replace(/\.+$/, "");
}

// ---------------------------------------------------------------------------
// Source badge helpers (merge card only)
// ---------------------------------------------------------------------------

export function sourceLabel(source: RuntimeSettingEntry["source"] | undefined): string {
  switch (source) {
    case "ui":
      return "UI";
    case "docker":
      return "Docker";
    default:
      return "Default";
  }
}

export function sourceClassName(source: RuntimeSettingEntry["source"] | undefined): string {
  switch (source) {
    case "ui":
      return "bg-[var(--ok)]/15 text-[var(--ok)]";
    case "docker":
      return "bg-[var(--warn)]/15 text-[var(--warn)]";
    default:
      return "bg-[var(--surface)] text-[var(--muted)]";
  }
}
