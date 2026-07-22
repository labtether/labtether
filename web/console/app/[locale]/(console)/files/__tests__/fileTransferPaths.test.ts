import { describe, expect, it } from "vitest";
import { joinTransferDestinationPath } from "../fileTransferPaths";

describe("joinTransferDestinationPath", () => {
  it.each([
    ["/", "/var/log/system.log", "/system.log"],
    ["/backup", "/var/log/system.log", "/backup/system.log"],
    ["~/backup/", "~/notes.txt", "~/backup/notes.txt"],
    ["C:\\Backup", "C:\\Users\\Michael\\report.txt", "C:\\Backup\\report.txt"],
    ["C:\\", "C:\\source.bin", "C:\\source.bin"],
  ])("joins %s and %s", (directory, sourcePath, expected) => {
    expect(joinTransferDestinationPath(directory, sourcePath)).toBe(expected);
  });
});
