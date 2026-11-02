import type { IDisposable, Terminal } from "@xterm/xterm";

/**
 * The Hub sends a persistent session's earlier output before its "ready" event, so a reconnect shows the history.
 * That history holds the terminal queries tmux and programs sent back then (device attributes, colour queries). If
 * the terminal answers them again, the answers reach today's shell as typed text: "0c1;2c0;276" piles up at the
 * prompt with every reconnect and breaks the next command (seen on the 2.0 lab Hub, 2026-11-02).
 *
 * write() feeds history: while those bytes are parsed, the queries get no answer. Live output written with
 * term.write() after "ready" is answered as usual. Colour and mode changes in the history still apply.
 */
export function createReplaySilencer(term: Terminal): { write: (data: string | Uint8Array) => void } & IDisposable {
  let pending = 0;
  const silent = () => pending > 0;
  const isQuery = (data: string) => silent() && data.split(";").includes("?");
  const parser = term.parser;
  const handlers: IDisposable[] = [
    parser.registerCsiHandler({ final: "c" }, silent), // DA1
    parser.registerCsiHandler({ prefix: ">", final: "c" }, silent), // DA2
    parser.registerCsiHandler({ final: "n" }, silent), // status and cursor position reports
    parser.registerCsiHandler({ prefix: "?", final: "n" }, silent),
    parser.registerCsiHandler({ intermediates: "$", final: "p" }, silent), // mode reports (DECRQM)
    parser.registerCsiHandler({ prefix: "?", intermediates: "$", final: "p" }, silent),
    parser.registerCsiHandler({ final: "t" }, silent), // window reports
    parser.registerDcsHandler({ intermediates: "$", final: "q" }, silent), // setting reports (DECRQSS)
    ...[4, 10, 11, 12].map((id) => parser.registerOscHandler(id, isQuery)), // colour queries ("?")
  ];
  return {
    write(data) {
      pending += 1;
      term.write(data, () => {
        pending -= 1;
      });
    },
    dispose() {
      for (const handler of handlers) handler.dispose();
    },
  };
}
