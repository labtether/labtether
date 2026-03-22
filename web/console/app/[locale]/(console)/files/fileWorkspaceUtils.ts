type ParentPathOptions = {
  fallbackRoot?: "~" | "/";
};

export function normalizePath(path: string): string {
  const trimmed = path.trim();
  if (trimmed === "") return "~";
  if (trimmed.length > 1 && trimmed.endsWith("/")) {
    return trimmed.replace(/\/+$/, "");
  }
  return trimmed;
}

export function joinPath(basePath: string, name: string): string {
  const cleanName = name.trim().replace(/^\/+/, "");
  if (cleanName === "") return basePath;
  if (basePath === "/") return `/${cleanName}`;
  if (basePath.endsWith("/")) return `${basePath}${cleanName}`;
  return `${basePath}/${cleanName}`;
}

export function parentPath(path: string, options?: ParentPathOptions): string {
  const normalized = normalizePath(path);
  if (normalized === "~" || normalized === "/") return normalized;
  const cut = normalized.lastIndexOf("/");
  if (cut < 0) {
    if (normalized.startsWith("~")) return "~";
    return options?.fallbackRoot ?? "~";
  }
  if (cut === 0) return "/";
  return normalized.slice(0, cut);
}

function pathWithinRoot(path: string, root: string): boolean {
  const normalizedPath = normalizePath(path);
  const normalizedRoot = normalizePath(root);
  if (normalizedRoot === "~") return true;
  if (normalizedRoot === "/") return normalizedPath.startsWith("/");
  if (normalizedPath === normalizedRoot) return true;
  return normalizedPath.startsWith(`${normalizedRoot}/`);
}

export function clampPathToRoot(path: string, root: string): string {
  const normalizedPath = normalizePath(path);
  const normalizedRoot = normalizePath(root);
  if (normalizedRoot === "~") return normalizedPath;
  if (normalizedPath === "~") return normalizedRoot;
  return pathWithinRoot(normalizedPath, normalizedRoot) ? normalizedPath : normalizedRoot;
}

export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "-";
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function formatTime(raw: string): string {
  if (!raw) return "-";
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) return raw;
  return parsed.toLocaleString();
}

export function triggerBlobDownload(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}
