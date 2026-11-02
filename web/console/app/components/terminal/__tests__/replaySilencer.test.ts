import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";

import { createReplaySilencer } from "../replaySilencer";

// Risk: on reconnect the Hub replays a tmux session's history, which holds tmux's old device-attribute and colour
// queries. Answering them typed "0c1;2c0;276" into the shell on every reconnect and broke the next command.
const QUERIES = "\x1b[c\x1b[>c\x1b]10;?\x1b\\\x1b]11;?\x1b\\\x1b[6n";

function setup() {
  const term = new Terminal({ allowProposedApi: true });
  const sent: string[] = [];
  term.onData((data) => sent.push(data));
  const history = createReplaySilencer(term);
  const flush = () => new Promise<void>((resolve) => term.write("", resolve));
  return { term, sent, history, flush };
}

describe("createReplaySilencer", () => {
  it("leaves queries in replayed history unanswered", async () => {
    const { sent, history, flush } = setup();
    history.write(QUERIES);
    history.write(new TextEncoder().encode(QUERIES));
    await flush();
    expect(sent).toEqual([]);
  });

  it("answers the same queries in live output", async () => {
    const { term, sent, history, flush } = setup();
    history.write("old output\r\n");
    term.write("\x1b[c\x1b[>c");
    await flush();
    expect(sent.join("")).toMatch(/^\x1b\[\?[\d;]+c\x1b\[>[\d;]+c$/);
  });
});
