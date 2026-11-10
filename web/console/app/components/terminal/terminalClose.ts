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
