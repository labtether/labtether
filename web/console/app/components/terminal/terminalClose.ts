/**
 * How a terminal stream closed. `clean` is a normal end: the shell finished (someone typed `exit`) or the Hub ended
 * the session on purpose. Anything else is a connection that dropped. `reason` is the Hub's own close reason.
 */
export type TerminalClose = { clean: boolean; reason?: string };

/** A WebSocket close as the terminal's callers need it. Only code 1000 is a normal end. */
export function terminalCloseFrom(event: { code: number; reason: string }): TerminalClose {
  const clean = event.code === 1000;
  return { clean, reason: event.reason || (clean ? undefined : "The connection closed unexpectedly") };
}

/**
 * The shell itself finished: someone typed `exit`, or the session was removed. A session that another window took
 * over closes cleanly too, and has not ended.
 */
export function isShellEnded(close: TerminalClose): boolean {
  return close.clean && !/attached elsewhere/i.test(close.reason ?? "");
}

/** The Hub closes a device's shell with this reason when the device's agent drops off the Hub. */
export function isDeviceGone(close: TerminalClose): boolean {
  return !close.clean && close.reason?.trim().toLowerCase() === "agent disconnected";
}
