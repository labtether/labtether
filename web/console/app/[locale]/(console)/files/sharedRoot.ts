import { listFiles } from "./fileOpsClient";
import { normalizePath } from "./remotePaths";

// The folder a device shares is its home folder ("home", the default) or the whole file system ("full"). The Files
// page needs it to know where Up one folder stops and which parts of the path can be opened.

type RootMode = "home" | "full";

/**
 * What the device's agent says it shares, or null when it has not said (an older agent, or the Hub could not be
 * asked). Only the agent's own report counts: the Hub's default is not what an agent set up by hand runs with.
 */
export async function reportedRootMode(assetId: string): Promise<RootMode | null> {
  try {
    const response = await fetch(`/api/agents/${encodeURIComponent(assetId)}/settings`, { cache: "no-store" });
    if (!response.ok) return null;
    const payload = (await response.json()) as { settings?: Array<{ key?: string; state_value?: string }> } | null;
    const reported = payload?.settings?.find((setting) => setting.key === "files_root_mode")?.state_value?.trim().toLowerCase();
    return reported === "home" || reported === "full" ? reported : null;
  } catch {
    return null;
  }
}

/**
 * The folder the device shares, given its home folder as the agent resolved it. A home-only agent refuses a list of
 * "/", which used to show as one refused request every time a device was opened. So the agent's setting is read
 * first, and "/" is asked for only when the agent shares the whole file system or has not said what it shares.
 */
export async function sharedRootOf(assetId: string, homePath: string, showHidden: boolean): Promise<string> {
  if ((await reportedRootMode(assetId)) === "home") return homePath;
  try {
    const data = await listFiles(assetId, "/", { showHidden, fallbackError: (status) => `Failed (${status})` });
    return normalizePath(data.path || "/");
  } catch {
    return homePath;
  }
}
