export function parseNeverMergeRuleLine(line: string): [string, string] | null {
  const separators = ["<=>", "=>", ","];
  const trimmed = line.trim();
  if (!trimmed) {
    return null;
  }
  for (const separator of separators) {
    if (!trimmed.includes(separator)) {
      continue;
    }
    const parts = trimmed.split(separator);
    if (parts.length !== 2) {
      return null;
    }
    const left = parts[0].trim();
    const right = parts[1].trim();
    if (!left || !right) {
      return null;
    }
    return [left, right];
  }
  return null;
}

export function canonicalURLIdentity(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed) {
    return null;
  }

  const parseURL = (value: string): URL | null => {
    try {
      return new URL(value);
    } catch {
      return null;
    }
  };

  let parsed = parseURL(trimmed);
  if (!parsed) {
    if (trimmed.includes("://")) {
      return null;
    }
    parsed = parseURL(`http://${trimmed}`);
    if (!parsed) {
      return null;
    }
  }

  const host = parsed.hostname.trim().toLowerCase();
  if (!host) {
    return null;
  }
  const scheme = (parsed.protocol || "http:").replace(/:$/, "").toLowerCase() || "http";
  const port = parsed.port || (scheme === "https" ? "443" : "80");
  const path = parsed.pathname && parsed.pathname.trim() !== "" ? parsed.pathname : "/";
  return `${scheme}://${host}:${port}${path}`;
}

export function mergePairKey(left: string, right: string): string | null {
  const a = left.trim();
  const b = right.trim();
  if (!a || !b || a === b) {
    return null;
  }
  return a < b ? `${a}||${b}` : `${b}||${a}`;
}

export function appendNeverMergeRule(existingRaw: string, leftURL: string, rightURL: string): string {
  const leftIdentity = canonicalURLIdentity(leftURL);
  const rightIdentity = canonicalURLIdentity(rightURL);
  if (!leftIdentity || !rightIdentity) {
    throw new Error("Both URLs must be valid to undo a merge.");
  }
  const incomingKey = mergePairKey(leftIdentity, rightIdentity);
  if (!incomingKey) {
    throw new Error("Cannot create a never-merge rule for identical URLs.");
  }

  const lines = existingRaw
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);

  const normalized: string[] = [];
  const seen = new Set<string>();
  for (const line of lines) {
    const parsedRule = parseNeverMergeRuleLine(line);
    if (!parsedRule) {
      normalized.push(line);
      continue;
    }
    const [left, right] = parsedRule;
    const parsedLeft = canonicalURLIdentity(left);
    const parsedRight = canonicalURLIdentity(right);
    if (!parsedLeft || !parsedRight) {
      normalized.push(line);
      continue;
    }
    const key = mergePairKey(parsedLeft, parsedRight);
    if (!key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    normalized.push(line);
  }

  if (seen.has(incomingKey)) {
    return normalized.join("\n");
  }

  const nextRule = `${leftURL.trim()} => ${rightURL.trim()}`;
  normalized.push(nextRule);
  return normalized.join("\n");
}
