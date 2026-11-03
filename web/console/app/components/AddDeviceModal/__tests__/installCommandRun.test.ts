// @vitest-environment node
import { spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { defaultLinuxInstallOptions, linuxInstallerCommand, manualInstallCommand } from "../agentInstallCommands";

// Runs the copied commands the way a person does: pasted into a shell whose input is where they type the token.
// curl and sudo are stand-ins; the "installer" and "agent" record the token file they were given.
let dir = "";

function stub(name: string, body: string) {
  const file = path.join(dir, "bin", name);
  writeFileSync(file, `#!/bin/sh\n${body}\n`);
  chmodSync(file, 0o755);
}

function runPasted(command: string) {
  return spawnSync("/bin/sh", ["-c", command], {
    cwd: dir,
    input: "lt-enroll-token-123\n",
    env: { PATH: `${path.join(dir, "bin")}:${process.env.PATH}`, OUT: path.join(dir, "out"), HOME: dir },
    encoding: "utf8",
    timeout: 20_000,
  });
}

beforeEach(() => {
  dir = mkdtempSync(path.join(tmpdir(), "lt-install-"));
  spawnSync("mkdir", [path.join(dir, "bin")]);
  const installer = `#!/bin/sh\nwhile [ $# -gt 0 ]; do [ "$1" = --enrollment-token-file ] && cat "$2" > "$OUT"; shift; done`;
  // curl ... -o FILE: write the stand-in installer to FILE.
  stub("curl", `while [ $# -gt 0 ]; do [ "$1" = -o ] && printf '%s\\n' '${installer.replace(/'/g, `'"'"'`)}' > "$2"; shift; done`);
  stub("sudo", 'exec "$@"');
  writeFileSync(path.join(dir, "labtether-agent"), '#!/bin/sh\ncat "$LABTETHER_ENROLLMENT_TOKEN_FILE" > "$OUT"\n');
  chmodSync(path.join(dir, "labtether-agent"), 0o755);
});

afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

describe("pasted agent install commands", () => {
  it("hands the installer the token the person typed", () => {
    const result = runPasted(linuxInstallerCommand("https://hub.example.com", defaultLinuxInstallOptions));

    expect(result.status, result.stderr).toBe(0);
    expect(readFileSync(path.join(dir, "out"), "utf8")).toBe("lt-enroll-token-123\n");
  });

  it("hands a manually started agent the token the person typed", () => {
    const result = runPasted(manualInstallCommand("linux", "wss://hub.example.com/ws/agent"));

    expect(result.status, result.stderr).toBe(0);
    expect(readFileSync(path.join(dir, "out"), "utf8")).toBe("lt-enroll-token-123\n");
  });
});
