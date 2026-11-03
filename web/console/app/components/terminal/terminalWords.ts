"use client";

import { useTranslations } from "next-intl";
import { useCallback } from "react";

// The session code (useSession, XTerminal) reports its own steps and failures in English; these are the ones the
// console writes itself. Text the Hub sends (a device's own refusal, a hop host) is shown as it arrives.
const OWN: Record<string, string> = {
  "Creating session...": "creatingSession",
  "Reconnecting session...": "reconnectingSession",
  "Requesting terminal stream ticket...": "requestingTicket",
  "Opening secure stream...": "openingStream",
  "Starting remote shell...": "startingShell",
  "Connected": "connected",
  "Terminal connection error": "connectionError",
  "The connection closed unexpectedly": "closedUnexpectedly",
  "Connection failed": "connectionFailed",
};

/** The console's own session words, in the person's language; anything else unchanged. */
export function useTerminalWords(): (text: string) => string {
  const t = useTranslations("terminal.words");
  return useCallback((text: string) => {
    const key = OWN[text.trim()];
    return key ? t(key) : text;
  }, [t]);
}
