export interface DesktopPrerequisiteGuide {
  key: string;
  dependency: string;
  title: string;
  summary: string;
  installCommands: string[];
  verifyCommand: string;
  rawReason: string;
}

export function parseDesktopPrerequisiteGuide(errorMessage: string | null | undefined): DesktopPrerequisiteGuide | null {
  const normalizedReason = normalizeCloseReason(errorMessage);
  if (!normalizedReason) {
    return null;
  }

  const missingToolMatch = normalizedReason.match(/([A-Za-z0-9._-]+)\s+not found(?:\s*:\s*install with\b.*)?$/i);
  if (!missingToolMatch) {
    return null;
  }

  const dependency = missingToolMatch[1];
  const installCommands = extractInstallCommands(normalizedReason);
  const resolvedCommands = installCommands.length > 0 ? installCommands : defaultInstallCommands(dependency);

  return {
    key: `${dependency.toLowerCase()}:${normalizedReason.toLowerCase()}`,
    dependency,
    title: `Install ${dependency} to enable VNC`,
    summary: `VNC could not start because ${dependency} is missing on this device.`,
    installCommands: resolvedCommands,
    verifyCommand: defaultVerifyCommand(dependency),
    rawReason: normalizedReason,
  };
}

function normalizeCloseReason(errorMessage: string | null | undefined): string {
  const trimmed = (errorMessage ?? "").trim();
  if (!trimmed) {
    return "";
  }

  // noVNC connection failures often arrive as:
  // "Connection closed (code: 1013, reason: <close-reason>)"
  const reasonMatch = trimmed.match(/reason:\s*(.+?)(?:\)|$)/i);
  if (reasonMatch?.[1]) {
    return reasonMatch[1].trim();
  }

  return trimmed;
}

function extractInstallCommands(reason: string): string[] {
  const installWithIndex = reason.toLowerCase().indexOf("install with");
  if (installWithIndex < 0) {
    return [];
  }

  const installSegment = reason.slice(installWithIndex + "install with".length).trim();
  if (!installSegment) {
    return [];
  }

  const quotedCommands = Array.from(installSegment.matchAll(/'([^']+)'/g))
    .map((match) => match[1].trim())
    .filter(Boolean);
  if (quotedCommands.length > 0) {
    return dedupeCommands(quotedCommands);
  }

  const splitCommands = installSegment
    .split(/\s+or\s+/i)
    .map(cleanInstallCommand)
    .filter(Boolean);
  return dedupeCommands(splitCommands);
}

function cleanInstallCommand(value: string): string {
  return value.replace(/^[,.;\s]+|[,.;\s]+$/g, "");
}

function dedupeCommands(commands: string[]): string[] {
  const seen = new Set<string>();
  const deduped: string[] = [];
  for (const cmd of commands) {
    const key = cmd.toLowerCase();
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    deduped.push(cmd);
  }
  return deduped;
}

function defaultInstallCommands(dependency: string): string[] {
  switch (dependency.toLowerCase()) {
    case "x11vnc":
      return [
        "apt install x11vnc",
        "pkg install x11vnc",
      ];
    case "xvfb":
      return ["apt install xvfb"];
    default:
      return [`install ${dependency}`];
  }
}

function defaultVerifyCommand(dependency: string): string {
  switch (dependency.toLowerCase()) {
    case "x11vnc":
      return "x11vnc -version";
    case "xvfb":
      return "Xvfb -help";
    default:
      return `${dependency} --version`;
  }
}
