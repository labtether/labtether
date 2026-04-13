// @vitest-environment node

import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

const mutatingHandlerPattern = /export\s+async\s+function\s+(POST|PUT|PATCH|DELETE)\s*\(/g;
const proxyAuthCall = "isMutationRequestOriginAllowed(request)";
const routeRoot = path.join(process.cwd(), "app/api");

function listRouteFiles(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      listRouteFiles(fullPath, out);
      continue;
    }
    if (entry.isFile() && entry.name === "route.ts") {
      out.push(fullPath);
    }
  }
  return out;
}

function findMatchingBrace(source: string, openBraceIndex: number): number {
  let depth = 0;
  let inSingleQuote = false;
  let inDoubleQuote = false;
  let inTemplate = false;
  let inLineComment = false;
  let inBlockComment = false;

  for (let i = openBraceIndex; i < source.length; i += 1) {
    const char = source[i];
    const next = source[i + 1];
    const prev = source[i - 1];

    if (inLineComment) {
      if (char === "\n") {
        inLineComment = false;
      }
      continue;
    }

    if (inBlockComment) {
      if (prev === "*" && char === "/") {
        inBlockComment = false;
      }
      continue;
    }

    if (inSingleQuote) {
      if (char === "'" && prev !== "\\") {
        inSingleQuote = false;
      }
      continue;
    }

    if (inDoubleQuote) {
      if (char === "\"" && prev !== "\\") {
        inDoubleQuote = false;
      }
      continue;
    }

    if (inTemplate) {
      if (char === "`" && prev !== "\\") {
        inTemplate = false;
      }
      continue;
    }

    if (char === "/" && next === "/") {
      inLineComment = true;
      i += 1;
      continue;
    }

    if (char === "/" && next === "*") {
      inBlockComment = true;
      i += 1;
      continue;
    }

    if (char === "'") {
      inSingleQuote = true;
      continue;
    }

    if (char === "\"") {
      inDoubleQuote = true;
      continue;
    }

    if (char === "`") {
      inTemplate = true;
      continue;
    }

    if (char === "{") {
      depth += 1;
      continue;
    }

    if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        return i;
      }
    }
  }

  return -1;
}

type MissingGuard = {
  file: string;
  method: string;
};

function findMutatingRoutesMissingOriginGuard(): MissingGuard[] {
  const missing: MissingGuard[] = [];

  for (const file of listRouteFiles(routeRoot)) {
    const source = fs.readFileSync(file, "utf8");
    if (!source.includes("backendAuthHeadersWithCookie(")) {
      continue;
    }

    for (const match of source.matchAll(mutatingHandlerPattern)) {
      const method = match[1];
      const handlerStart = match.index ?? -1;
      if (handlerStart < 0) {
        continue;
      }

      const declaration = match[0];
      const openBraceOffset = declaration.lastIndexOf("{");
      const openBraceIndex =
        openBraceOffset >= 0 ? handlerStart + openBraceOffset : -1;
      if (openBraceIndex < 0) {
        continue;
      }

      const closeBraceIndex = findMatchingBrace(source, openBraceIndex);
      if (closeBraceIndex < 0) {
        continue;
      }

      const handlerBody = source.slice(openBraceIndex + 1, closeBraceIndex);
      if (!handlerBody.includes(proxyAuthCall)) {
        missing.push({
          file: path.relative(process.cwd(), file),
          method,
        });
      }
    }
  }

  return missing;
}

describe("authenticated mutating console proxies", () => {
  it("enforce same-origin checks before proxying mutations", () => {
    const missing = findMutatingRoutesMissingOriginGuard();
    expect(missing).toEqual([]);
  });
});
