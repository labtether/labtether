type TmuxAsset = { platform?: string; metadata?: Record<string, string> } | null | undefined;

/** A Windows device: tmux does not run under its shells (PowerShell or cmd). */
export function isWindowsDevice(asset: TmuxAsset): boolean {
  const meta = asset?.metadata ?? {};
  const words = `${asset?.platform ?? ""} ${meta.platform ?? ""} ${meta.os ?? ""} ${meta.os_pretty_name ?? meta.os_name ?? ""}`;
  return words.toLowerCase().includes("windows");
}

/**
 * The package manager command that installs tmux on the device's OS, or null when there is none to offer: tmux does
 * not run under a Windows shell (PowerShell or cmd), so a Windows device gets no install command.
 */
export function tmuxInstallCommand(asset: TmuxAsset): string | null {
  const meta = asset?.metadata ?? {};
  const platform = `${asset?.platform ?? ""} ${meta.platform ?? ""} ${meta.os ?? ""}`.toLowerCase();
  const osId = (meta.os_id ?? "").toLowerCase();
  const like = (meta.os_id_like ?? "").toLowerCase();
  const name = (meta.os_pretty_name ?? meta.os_name ?? "").toLowerCase();
  if (isWindowsDevice(asset)) return null;
  if (platform.includes("darwin") || name.includes("macos") || name.includes("darwin")) return "brew install tmux";
  if (["fedora", "rhel", "centos", "rocky", "alma"].includes(osId) || like.includes("fedora") || like.includes("rhel")) return "sudo dnf install -y tmux";
  if (["arch", "manjaro"].includes(osId) || like.includes("arch")) return "sudo pacman -S --noconfirm tmux";
  if (["opensuse", "sles"].includes(osId) || like.includes("suse")) return "sudo zypper install -y tmux";
  if (osId === "alpine") return "sudo apk add tmux";
  return "sudo apt install -y tmux";
}
