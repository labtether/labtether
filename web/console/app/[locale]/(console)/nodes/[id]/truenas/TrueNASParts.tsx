"use client";

// Small parts shared by the TrueNAS views: the row switch, the load/empty states of a list sheet and the words for
// ZFS health states.

import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { DiEmpty, DiNote, DiStatus } from "../proxmox/DiList";
import { failureText, type Tone } from "./truenasModel";

/** A 28×16 on/off switch for a row. */
export function TnSwitch({ on, label, onToggle, disabled = false }: { on: boolean; label: string; onToggle: () => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      className={`di-tn-switch${on ? " on" : ""}`}
      disabled={disabled}
      onClick={onToggle}
    >
      <i />
    </button>
  );
}

/**
 * What a list sheet shows before it has rows: the load failure with Try again, "Loading", or the empty line.
 * Returns null once there are rows to show.
 */
export function TnListState({ loading, error, count, empty, failed, onRetry }: {
  loading: boolean;
  error: string | null;
  count: number;
  empty: ReactNode;
  failed: (reason: string) => string;
  onRetry: () => void;
}) {
  const k = useTranslations("devices-proxmox.kit");
  const t = useTranslations("devices-truenas.common");
  if (count > 0) return null;
  if (error) {
    return (
      <DiEmpty
        title={failed(failureText(error, t("noAnswer")))}
        action={<Button size="sm" onClick={onRetry}>{k("retry")}</Button>}
      />
    );
  }
  return <DiEmpty title={loading ? k("loading") : empty} />;
}

/** A state word and its time; the dot shows only when the state is not fine. */
export function TnWord({ tone, word, time }: { tone: Tone; word: ReactNode; time?: string }) {
  if (tone && tone !== "ok") return <DiStatus tone={tone} word={word} time={time || undefined} />;
  return (
    <span className="di-st">
      <span>{word}</span>
      {time ? <span className="di-st-t">{time}</span> : null}
    </span>
  );
}

/** A failed action, above the list it belongs to. */
export function TnFailure({ message }: { message: string | null }) {
  if (!message) return null;
  return <DiNote tone="bad">{message}</DiNote>;
}

const HEALTH_WORDS = new Set(["ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED"]);

/** "Degraded", "Faulted" …; other states as TrueNAS reported them. */
export function useHealthWord() {
  const t = useTranslations("devices-truenas.health");
  return (status: string) => {
    const value = status.trim().toUpperCase();
    return HEALTH_WORDS.has(value) ? t(value.toLowerCase()) : status;
  };
}
