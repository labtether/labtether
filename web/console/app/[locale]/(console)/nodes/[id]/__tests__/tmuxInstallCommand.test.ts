import { expect, it } from "vitest";
import { tmuxInstallCommand } from "../tmuxInstallCommand";

type Case = [Parameters<typeof tmuxInstallCommand>[0], string | null];

// Risk: a device is told to run another OS's package manager. A Windows device was shown "sudo apt install -y tmux",
// and tmux does not run under a Windows shell at all.
const cases: Case[] = [
  [{ platform: "windows", metadata: { os_name: "Microsoft Windows 11 Pro" } }, null],
  [{ metadata: { platform: "windows" } }, null],
  [{ platform: "darwin", metadata: { os_name: "darwin" } }, "brew install tmux"],
  [{ platform: "linux", metadata: { os_id: "fedora" } }, "sudo dnf install -y tmux"],
  [{ platform: "linux", metadata: { os_id: "debian" } }, "sudo apt install -y tmux"],
];

it.each(cases)("offers the install command of the device's own OS (%j)", (asset, command) => {
  expect(tmuxInstallCommand(asset)).toBe(command);
});
