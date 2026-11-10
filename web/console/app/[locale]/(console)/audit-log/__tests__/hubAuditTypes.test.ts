import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import en from "../../../../../messages/en/audit-log.json";
import { actionKey, KNOWN_ACTIONS } from "../auditModel";

// Risk: the Hub starts writing a new kind of audit row and the Audit log shows its type as raw words ("Alert
// acknowledged"), because nobody added a sentence. This reads the Hub's Go source beside the console (the same
// repository) and collects every audit type it can write: a dotted lower-case string on a line that names audit
// (audit.NewEvent, an ...AuditType constant, an audit helper call), or the Type of an audit.Event literal.
// When it fails: add the type to KNOWN_ACTIONS and its sentence to audit-log.json in the five languages. If the
// string is not an audit type, add it to NOT_TYPES with the reason.
const hub = path.resolve(process.cwd(), "../..");
const DOTTED = /"([a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+)"/g;
const TYPE_FIELD = /\bType:\s*"([a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+)"/;
const NOT_TYPES = new Set<string>();

function goFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return goFiles(full);
    return entry.name.endsWith(".go") && !entry.name.endsWith("_test.go") ? [full] : [];
  });
}

function hubAuditTypes(): Map<string, string> {
  const found = new Map<string, string>();
  for (const file of [...goFiles(path.join(hub, "internal")), ...goFiles(path.join(hub, "cmd"))]) {
    const source = readFileSync(file, "utf8");
    if (!/audit/i.test(source)) continue;
    const hasEvent = source.includes("audit.Event{");
    for (const line of source.split("\n")) {
      if (/audit/i.test(line)) for (const match of line.matchAll(DOTTED)) found.set(match[1], path.relative(hub, file));
      const type = hasEvent ? TYPE_FIELD.exec(line) : null;
      if (type) found.set(type[1], path.relative(hub, file));
    }
  }
  for (const not of NOT_TYPES) found.delete(not);
  return found;
}

// A console checked out without the Hub source (a web/console-only copy) has nothing to compare with.
describe.skipIf(!existsSync(path.join(hub, "internal", "audit")))("Hub audit types", () => {
  it("each has a sentence in the Audit log", () => {
    const types = hubAuditTypes();
    // The scan itself must keep working: these are written in three different ways in the Hub.
    for (const sample of ["api.request", "asset.updated", "auth.user.created", "terminal.session.created"]) {
      expect(types.has(sample), `the scan no longer finds ${sample}`).toBe(true);
    }
    const sentences = en.actions as Record<string, string>;
    const missing = [...types].filter(([type]) => !KNOWN_ACTIONS.has(type) || !sentences[actionKey(type)]).map(([type, file]) => `${type} (${file})`);
    expect(missing).toEqual([]);
  });
});
