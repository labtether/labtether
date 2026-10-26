#!/usr/bin/env node
// Reports translation gaps for every console locale against English.
// Fails when a locale misses a namespace file or key, carries a key that
// English no longer has, or changes the ICU arguments of a message.
// Usage: node scripts/check-i18n-keys.mjs [--list] [--identical]
import { readdirSync, readFileSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const SOURCE_LOCALE = "en";

export function flattenMessages(value, prefix = "", out = {}) {
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (child && typeof child === "object" && !Array.isArray(child)) {
      flattenMessages(child, path, out);
    } else {
      out[path] = child;
    }
  }
  return out;
}

// Returns the sorted ICU argument signature of a message, e.g.
// "{count, plural, one {# x} other {# y}}" -> ["count:plural"].
// Plural/select branch text is scanned for nested arguments only.
export function icuArguments(message) {
  if (typeof message !== "string") return [];
  const found = new Set();
  scanText(message, 0, found, false);
  return [...found].sort();
}

function scanText(text, start, found, nested) {
  let i = start;
  while (i < text.length) {
    const ch = text[i];
    if (ch === "}" && nested) return i;
    if (ch === "{") {
      i = scanArgument(text, i + 1, found);
    } else {
      i += 1;
    }
  }
  return i;
}

function scanArgument(text, start, found) {
  const close = findArgumentEnd(text, start);
  const header = text.slice(start, close).split(",");
  const name = header[0].trim();
  const kind = (header[1] ?? "").trim();
  if (!/^[A-Za-z0-9_]+$/.test(name)) return close + 1;
  found.add(kind ? `${name}:${kind}` : name);
  if (kind === "plural" || kind === "select" || kind === "selectordinal") {
    return scanBranches(text, start, found);
  }
  return close + 1;
}

function findArgumentEnd(text, start) {
  let i = start;
  while (i < text.length && text[i] !== "}" && text[i] !== "{") i += 1;
  return i;
}

function scanBranches(text, start, found) {
  let commas = 0;
  let i = start;
  while (i < text.length && commas < 2) {
    if (text[i] === ",") commas += 1;
    i += 1;
  }
  while (i < text.length) {
    const ch = text[i];
    if (ch === "}") return i + 1;
    if (ch === "{") {
      i = scanText(text, i + 1, found, true) + 1;
    } else {
      i += 1;
    }
  }
  return i;
}

function readLocale(messagesDir, locale) {
  const dir = join(messagesDir, locale);
  const namespaces = {};
  if (!existsSync(dir)) return namespaces;
  for (const file of readdirSync(dir).filter((f) => f.endsWith(".json"))) {
    const raw = JSON.parse(readFileSync(join(dir, file), "utf8"));
    namespaces[file.replace(/\.json$/, "")] = flattenMessages(raw);
  }
  return namespaces;
}

export function compareLocales(messagesDir) {
  const source = readLocale(messagesDir, SOURCE_LOCALE);
  const locales = readdirSync(messagesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && entry.name !== SOURCE_LOCALE)
    .map((entry) => entry.name)
    .sort();
  const sourceTotal = Object.values(source).reduce((n, ns) => n + Object.keys(ns).length, 0);

  return locales.map((locale) => {
    const target = readLocale(messagesDir, locale);
    const report = {
      locale,
      sourceTotal,
      missingFiles: [],
      missing: [],
      extra: [],
      argumentMismatch: [],
      identical: [],
    };
    for (const [ns, sourceKeys] of Object.entries(source)) {
      const targetKeys = target[ns];
      if (!targetKeys) report.missingFiles.push(ns);
      for (const [key, value] of Object.entries(sourceKeys)) {
        const id = `${ns}:${key}`;
        if (!targetKeys || !(key in targetKeys)) {
          report.missing.push(id);
          continue;
        }
        const want = icuArguments(value).join(",");
        const got = icuArguments(targetKeys[key]).join(",");
        if (want !== got) report.argumentMismatch.push(`${id} (en: ${want || "none"}, ${locale}: ${got || "none"})`);
        if (targetKeys[key] === value && /[a-z]{3,}/.test(String(value))) report.identical.push(id);
      }
    }
    for (const [ns, targetKeys] of Object.entries(target)) {
      for (const key of Object.keys(targetKeys)) {
        if (!source[ns] || !(key in source[ns])) report.extra.push(`${ns}:${key}`);
      }
    }
    return report;
  });
}

function main(argv) {
  const here = dirname(fileURLToPath(import.meta.url));
  const messagesDir = join(here, "..", "messages");
  const list = argv.includes("--list");
  const showIdentical = argv.includes("--identical");
  let failed = false;

  for (const report of compareLocales(messagesDir)) {
    const translated = report.sourceTotal - report.missing.length;
    const problems =
      report.missingFiles.length + report.missing.length + report.extra.length + report.argumentMismatch.length;
    failed ||= problems > 0;
    console.log(
      `${report.locale}: ${translated}/${report.sourceTotal} keys, ` +
        `${report.missing.length} missing (${report.missingFiles.length} files), ` +
        `${report.extra.length} extra, ${report.argumentMismatch.length} argument mismatches` +
        (showIdentical ? `, ${report.identical.length} identical to en` : ""),
    );
    const sections = [
      ["missing file", report.missingFiles],
      ["missing", list ? report.missing : []],
      ["extra", report.extra],
      ["argument mismatch", report.argumentMismatch],
      ["identical to en", showIdentical ? report.identical : []],
    ];
    for (const [label, items] of sections) {
      for (const item of items) console.log(`  ${label}: ${item}`);
    }
  }
  if (failed) {
    console.error("i18n check failed: translate the keys above or rerun with --list for every missing key.");
    process.exit(1);
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  main(process.argv.slice(2));
}
