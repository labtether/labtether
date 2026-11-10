import { trimLeadingChars, trimTrailingChars, trimTrailingSlashes } from "../../../../lib/trimChars";

// Paths on the machine a file view browses, in the two forms an agent answers with. Linux, macOS and FreeBSD agents
// and every saved connection use "/" ("/home/demo/docs"). A Windows agent answers with a drive path
// ("C:\Users\demo\docs") or a UNC share ("\\nas01\share\docs"); it takes either slash and matches names in any case.
// "~" is the agent's home folder on every platform, and an agent expands "~/docs" only, never "~\docs".
//
// The style is read from a path the agent returned, or from the device's platform while only "~" is known. It is
// never guessed from the browser's own OS. Shared by the Files page, a device's Files tab and the Remote View files
// drawer.

export type PathStyle = "posix" | "windows";

export type Crumb = { label: string; path: string };

export type DevicePlace = { id: "home" | "root"; path: string };

type ParentPathOptions = {
  fallbackRoot?: "~" | "/";
};

type WindowsParts = { root: string; names: string[] };

const DRIVE = /^[A-Za-z]:(?:[\\/]|$)/;
const DRIVE_ROOT = /^[A-Za-z]:\\$/;

export function isWindowsPlatform(platform: string | null | undefined): boolean {
  return /windows/i.test(platform ?? "");
}

/** "windows" for a drive or UNC path, "posix" for one that starts with "/". "~" and other paths follow the platform. */
export function pathStyleOf(path: string, platform?: string | null): PathStyle {
  const trimmed = path.trim();
  if (DRIVE.test(trimmed) || trimmed.startsWith("\\\\")) return "windows";
  if (trimmed.startsWith("/")) return "posix";
  return isWindowsPlatform(platform) ? "windows" : "posix";
}

/** "C:/Users/demo/" is root "C:\" and names Users, demo; "\\nas01\share\docs" is root "\\nas01\share\" and docs. */
function windowsParts(path: string): WindowsParts {
  const raw = path.trim().replaceAll("/", "\\");
  const split = (rest: string) => rest.split("\\").filter(Boolean);
  if (DRIVE.test(raw)) return { root: `${raw.slice(0, 2)}\\`, names: split(raw.slice(2)) };
  if (raw.startsWith("\\\\")) {
    const [server = "", share = "", ...names] = split(raw);
    return share ? { root: `\\\\${server}\\${share}\\`, names } : { root: `\\\\${server}`, names: [] };
  }
  if (raw === "~" || raw.startsWith("~\\")) return { root: "~", names: split(raw.slice(1)) };
  // No drive and no share: the agent reads "\docs" and "docs" alike, as a folder in the one it shares.
  return { root: raw.startsWith("\\") ? "\\" : "", names: split(raw) };
}

/** A whole drive or share: "C:\" or "\\nas01\share\". */
const isVolumeRoot = ({ root, names }: WindowsParts) => names.length === 0 && root.length > 1 && root.endsWith("\\");

function windowsPath({ root, names }: WindowsParts): string {
  if (root === "~") return names.length > 0 ? `~/${names.join("/")}` : "~";
  return `${root}${names.join("\\")}`;
}

/**
 * A path without a trailing separator; an empty one is "~". A Windows path also gets backslashes throughout and its
 * root in full ("C:" becomes "C:\"); a "~" path keeps "/" because the agent expands no other form.
 */
export function normalizePath(path: string, style: PathStyle = pathStyleOf(path)): string {
  const trimmed = path.trim();
  if (trimmed === "") return "~";
  if (style === "windows") return windowsPath(windowsParts(trimmed));
  if (trimmed.length > 1 && trimmed.endsWith("/")) return trimTrailingSlashes(trimmed) || "/";
  return trimmed;
}

/** What a person typed in the path field, in the form the agent takes. Only a Windows path is rewritten. */
export function typedPath(input: string, style: PathStyle): string {
  const trimmed = input.trim();
  return style === "windows" && trimmed !== "" ? normalizePath(trimmed, "windows") : trimmed;
}

export function joinPath(basePath: string, name: string): string {
  const windows = pathStyleOf(basePath) === "windows";
  const cleanName = trimLeadingChars(name.trim(), windows ? "\\/" : "/");
  if (cleanName === "") return basePath;
  if (windows) {
    const base = normalizePath(basePath, "windows");
    return base.endsWith("\\") ? `${base}${cleanName}` : `${base}\\${cleanName}`;
  }
  if (basePath === "/") return `/${cleanName}`;
  if (basePath.endsWith("/")) return `${basePath}${cleanName}`;
  return `${basePath}/${cleanName}`;
}

/** The folder above. A root ("/", "~", "C:\", "\\nas01\share\") is its own parent. */
export function parentPath(path: string, options?: ParentPathOptions): string {
  const normalized = normalizePath(path);
  if (pathStyleOf(normalized) === "windows") {
    const { root, names } = windowsParts(normalized);
    return windowsPath({ root, names: names.slice(0, -1) });
  }
  if (normalized === "~" || normalized === "/") return normalized;
  const cut = normalized.lastIndexOf("/");
  if (cut < 0) {
    if (normalized.startsWith("~")) return "~";
    return options?.fallbackRoot ?? "~";
  }
  if (cut === 0) return "/";
  return normalized.slice(0, cut);
}

/** The last name in a path: "report.txt" for "C:\Users\demo\report.txt" and for "/home/demo/report.txt". */
export function baseName(path: string): string {
  const windows = pathStyleOf(path) === "windows";
  const trimmed = trimTrailingChars(path.trim(), windows ? "\\/" : "/");
  const cut = Math.max(trimmed.lastIndexOf("/"), windows ? trimmed.lastIndexOf("\\") : -1);
  return trimmed.slice(cut + 1);
}

/** The same folder: Windows paths match in any case and with either slash. */
export function samePath(left: string, right: string): boolean {
  const a = normalizePath(left);
  const b = normalizePath(right);
  if (pathStyleOf(a) !== "windows" && pathStyleOf(b) !== "windows") return a === b;
  return normalizePath(a, "windows").toLowerCase() === normalizePath(b, "windows").toLowerCase();
}

/** A root is its own parent, so Up one folder has nowhere to go. */
export function isTopFolder(path: string): boolean {
  const normalized = normalizePath(path);
  return normalized === "~" || samePath(parentPath(normalized, { fallbackRoot: "/" }), normalized);
}

/** The folder names from base down to path (none when they are the same folder), or null when path is not in base. */
export function namesBelow(path: string, base: string): string[] | null {
  const normalizedBase = normalizePath(base);
  if (pathStyleOf(normalizedBase) === "windows") {
    const outer = windowsParts(normalizedBase);
    const inner = windowsParts(path);
    const lower = (value: string) => value.toLowerCase();
    if (lower(inner.root) !== lower(outer.root)) return null;
    if (outer.names.some((name, index) => lower(inner.names[index] ?? "") !== lower(name))) return null;
    return inner.names.slice(outer.names.length);
  }
  const normalizedPath = normalizePath(path);
  if (normalizedPath === normalizedBase) return [];
  const prefix = normalizedBase === "/" ? "/" : `${normalizedBase}/`;
  if (!normalizedPath.startsWith(prefix)) return null;
  return normalizedPath.slice(prefix.length).split("/").filter(Boolean);
}

const isHomePath = (path: string) => path === "~" || path.startsWith("~/");

/**
 * Whether the agent can be asked for path, given the folder it shares. A Windows device in full mode shares every
 * fixed drive, but its root reads as one of them ("C:\") and nothing lists the others. So with a drive as the root, a
 * path on any drive goes to the agent, which refuses the drives it does not share.
 */
function pathWithinRoot(path: string, root: string): boolean {
  if (root === "~") return true;
  if (DRIVE_ROOT.test(root) && DRIVE.test(path)) return true;
  return namesBelow(path, root) !== null;
}

/** Whether a typed path leaves the folder the agent shares. "~" and "~/docs" are the agent's own to resolve. */
export function isOutsideRoot(path: string, root: string): boolean {
  const normalizedRoot = normalizePath(root);
  const wanted = normalizePath(path, pathStyleOf(normalizedRoot));
  return !isHomePath(wanted) && !pathWithinRoot(wanted, normalizedRoot);
}

/** The path to ask the agent for: path itself inside the shared folder, else the shared folder. */
export function clampPathToRoot(path: string, root: string): string {
  const normalizedRoot = normalizePath(root);
  const wanted = normalizePath(path, pathStyleOf(normalizedRoot));
  return isHomePath(wanted) || pathWithinRoot(wanted, normalizedRoot) ? wanted : normalizedRoot;
}

/**
 * The path in an agent's refusal ('path "/etc" is outside the allowed base directory'), or null for other errors.
 * The agent quotes the path, so a Windows path arrives with doubled backslashes; they are read back as single ones.
 */
export function outsideBasePath(message: string | null | undefined): string | null {
  const match = /path "((?:[^"\\]|\\.)*)" is outside the allowed base directory/i.exec(message ?? "");
  return match ? match[1].replace(/\\(["\\])/g, "$1") : null;
}

/** The clickable parts of a path: "/", "var", "lib" for /var/lib; "~", "docs" for ~/docs; "C:", "Users" on Windows. */
export function pathCrumbs(path: string): Crumb[] {
  const raw = path.trim() || "~";
  if (pathStyleOf(raw) === "windows") {
    const { root, names } = windowsParts(raw);
    const label = DRIVE_ROOT.test(root) ? root.slice(0, 2) : trimTrailingChars(root, "\\");
    return [{ label, path: root }, ...names.map((name, i) => ({ label: name, path: `${root}${names.slice(0, i + 1).join("\\")}` }))];
  }
  if (raw.startsWith("~")) {
    const segs = raw.slice(1).split(/[\\/]/).filter(Boolean);
    return [{ label: "~", path: "~" }, ...segs.map((s, i) => ({ label: s, path: `~/${segs.slice(0, i + 1).join("/")}` }))];
  }
  const segs = raw.split("/").filter(Boolean);
  return [{ label: "/", path: "/" }, ...segs.map((s, i) => ({ label: s, path: `/${segs.slice(0, i + 1).join("/")}` }))];
}

/**
 * The folders a device's path bar offers to jump to. A Windows device has no "/": Root is the drive or share it
 * shares when that is known, and is left out when the device shares only a folder (Home opens it).
 */
export function devicePlaces(style: PathStyle, root?: string): DevicePlace[] {
  if (style !== "windows") return [{ id: "home", path: "~" }, { id: "root", path: "/" }];
  const places: DevicePlace[] = [{ id: "home", path: "~" }];
  const shared = root ? windowsParts(root) : null;
  if (shared && isVolumeRoot(shared)) places.push({ id: "root", path: shared.root });
  return places;
}
