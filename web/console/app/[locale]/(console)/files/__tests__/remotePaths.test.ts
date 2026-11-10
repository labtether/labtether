import { describe, expect, it } from "vitest";
import {
  baseName,
  clampPathToRoot,
  devicePlaces,
  isOutsideRoot,
  isTopFolder,
  joinPath,
  namesBelow,
  outsideBasePath,
  parentPath,
  pathCrumbs,
  pathStyleOf,
  samePath,
  typedPath,
} from "../remotePaths";

// Every case here was a fault on a real Windows 11 device, next to the same case on Linux to show it did not move.
const WIN_HOME = "C:\\Users\\demo";
const POSIX_HOME = "/home/demo";

describe("the style of a path", () => {
  it("comes from the agent's path, and from the platform only while the path is ~", () => {
    expect(pathStyleOf(WIN_HOME)).toBe("windows");
    expect(pathStyleOf("\\\\nas01\\share\\docs")).toBe("windows");
    expect(pathStyleOf(POSIX_HOME, "windows")).toBe("posix");
    expect(pathStyleOf("~", "windows")).toBe("windows");
    expect(pathStyleOf("~")).toBe("posix");
  });
});

// Risk: a folder inside a first-level folder was asked for as "C:\Users\demo\scratch/lvl2", failed the shared-folder
// check and was swapped for the home folder with no message.
describe("opening a folder", () => {
  it.each([
    [WIN_HOME, "scratch", "C:\\Users\\demo\\scratch"],
    ["C:\\Users\\demo\\scratch", "lvl2 ✓", "C:\\Users\\demo\\scratch\\lvl2 ✓"],
    ["C:\\", "Users", "C:\\Users"],
    ["\\\\nas01\\share\\", "docs", "\\\\nas01\\share\\docs"],
    [POSIX_HOME, "scratch", "/home/demo/scratch"],
    ["/home/demo/scratch", "lvl2", "/home/demo/scratch/lvl2"],
    ["/", "etc", "/etc"],
    ["~", "docs", "~/docs"],
  ])("joins %s and %s", (base, name, joined) => {
    expect(joinPath(base, name)).toBe(joined);
  });

  it("keeps a second-level folder inside the shared folder", () => {
    expect(clampPathToRoot(joinPath("C:\\Users\\demo\\scratch", "lvl2"), WIN_HOME)).toBe("C:\\Users\\demo\\scratch\\lvl2");
    expect(clampPathToRoot(joinPath("/home/demo/scratch", "lvl2"), POSIX_HOME)).toBe("/home/demo/scratch/lvl2");
  });
});

// Risk: Up one folder on a Windows path found no "/" and went to the home folder from any depth.
describe("going up one folder", () => {
  it.each([
    ["C:\\Users\\demo\\scratch\\lvl2 ✓", "C:\\Users\\demo\\scratch"],
    ["C:\\Users", "C:\\"],
    ["C:\\", "C:\\"],
    ["\\\\nas01\\share\\docs", "\\\\nas01\\share\\"],
    ["\\\\nas01\\share\\", "\\\\nas01\\share\\"],
    ["/home/demo/scratch/lvl2", "/home/demo/scratch"],
    ["/home", "/"],
    ["/", "/"],
    ["~/docs", "~"],
    ["~", "~"],
  ])("goes from %s to %s", (path, parent) => {
    expect(parentPath(path, { fallbackRoot: "/" })).toBe(parent);
  });

  it("stops at a drive, a share, / and ~", () => {
    expect(["C:\\", "\\\\nas01\\share\\", "/", "~"].map(isTopFolder)).toEqual([true, true, true, true]);
    expect(["C:\\Users", "\\\\nas01\\share\\docs", "/home"].map(isTopFolder)).toEqual([false, false, false]);
  });

  it("names the folders below the shared one, for the Remote View drawer", () => {
    expect(namesBelow("c:\\users\\demo\\scratch\\lvl2", WIN_HOME)).toEqual(["scratch", "lvl2"]);
    expect(namesBelow("/home/demo/scratch/lvl2", POSIX_HOME)).toEqual(["scratch", "lvl2"]);
    expect(namesBelow(WIN_HOME, WIN_HOME)).toEqual([]);
    expect(namesBelow("C:\\Users", WIN_HOME)).toBeNull();
  });
});

// Risk: every path typed for a Windows device was refused as outside the shared folder, and "~/folder" did nothing.
describe("a typed path", () => {
  it.each([
    ["C:\\Users\\demo\\scratch", "C:\\Users\\demo\\scratch"],
    ["C:/Users/demo/scratch", "C:\\Users\\demo\\scratch"],
    ["c:\\users\\DEMO\\scratch\\", "c:\\users\\DEMO\\scratch"],
    ["~/scratch", "~/scratch"],
    ["~\\scratch", "~/scratch"],
    ["~", "~"],
  ])("%s opens on a Windows device", (typed, sent) => {
    const wanted = typedPath(typed, "windows");
    expect(wanted).toBe(sent);
    expect(isOutsideRoot(wanted, WIN_HOME)).toBe(false);
    expect(clampPathToRoot(wanted, WIN_HOME)).toBe(sent);
  });

  it.each([
    ["/home/demo/scratch", "/home/demo/scratch"],
    ["/home/demo/scratch/", "/home/demo/scratch"],
    ["/home/demo/a\\b", "/home/demo/a\\b"],
    ["~/scratch", "~/scratch"],
  ])("%s opens on a Linux device", (typed, sent) => {
    const wanted = typedPath(typed, "posix");
    expect(wanted).toBe(typed);
    expect(isOutsideRoot(wanted, POSIX_HOME)).toBe(false);
    expect(clampPathToRoot(wanted, POSIX_HOME)).toBe(sent);
  });

  it.each([
    ["C:\\Windows", WIN_HOME],
    ["C:\\Users", WIN_HOME],
    ["C:\\Users\\demolition", WIN_HOME],
    ["M:\\", WIN_HOME],
    ["\\\\localhost\\C$\\Users", WIN_HOME],
    ["\\\\nas01\\share", "C:\\"],
    ["/etc", POSIX_HOME],
    ["/home", POSIX_HOME],
    ["/home/demolition", POSIX_HOME],
  ])("%s is outside %s", (path, root) => {
    expect(isOutsideRoot(path, root)).toBe(true);
    expect(clampPathToRoot(path, root)).toBe(root);
  });

  it("goes to the agent for any drive when the device shares whole drives", () => {
    expect(isOutsideRoot("D:\\data", "C:\\")).toBe(false);
    expect(clampPathToRoot("d:/data/", "C:\\")).toBe("d:\\data");
    expect(isOutsideRoot("/etc", "/")).toBe(false);
  });

  it("matches Windows folders in any case, and Linux folders exactly", () => {
    expect(samePath("c:\\users\\demo", "C:/Users/demo/")).toBe(true);
    expect(samePath("/home/Demo", POSIX_HOME)).toBe(false);
  });

  it("reads a UNC share as a root", () => {
    expect(typedPath("//nas01/share/docs/", "windows")).toBe("\\\\nas01\\share\\docs");
    expect(isOutsideRoot("\\\\NAS01\\Share\\docs\\2026", "\\\\nas01\\share\\docs")).toBe(false);
  });
});

// Crumbs decide which folder a click opens; a wrong path lists the wrong folder or leaves the home tree.
describe("pathCrumbs", () => {
  it("splits POSIX, home, drive and UNC paths into openable folders", () => {
    expect(pathCrumbs("/var/lib/vz")).toEqual([
      { label: "/", path: "/" },
      { label: "var", path: "/var" },
      { label: "lib", path: "/var/lib" },
      { label: "vz", path: "/var/lib/vz" },
    ]);
    expect(pathCrumbs("~/docs/")).toEqual([{ label: "~", path: "~" }, { label: "docs", path: "~/docs" }]);
    expect(pathCrumbs("C:\\Users\\demo")).toEqual([
      { label: "C:", path: "C:\\" },
      { label: "Users", path: "C:\\Users" },
      { label: "demo", path: "C:\\Users\\demo" },
    ]);
    expect(pathCrumbs("\\\\nas01\\share\\docs")).toEqual([
      { label: "\\\\nas01\\share", path: "\\\\nas01\\share\\" },
      { label: "docs", path: "\\\\nas01\\share\\docs" },
    ]);
    expect(pathCrumbs("")).toEqual([{ label: "~", path: "~" }]);
  });
});

describe("what the path bar and messages show", () => {
  it("offers Home and, for a Windows device that shares a whole drive, that drive", () => {
    expect(devicePlaces("posix", POSIX_HOME)).toEqual([{ id: "home", path: "~" }, { id: "root", path: "/" }]);
    expect(devicePlaces("windows", WIN_HOME)).toEqual([{ id: "home", path: "~" }]);
    expect(devicePlaces("windows", "C:\\")).toEqual([{ id: "home", path: "~" }, { id: "root", path: "C:\\" }]);
  });

  // Risk: a download from a Windows device is saved under its whole path instead of the file's name.
  it("takes the file's name from either kind of path", () => {
    expect(baseName("C:\\Users\\demo\\scratch\\ünï ✓.bin")).toBe("ünï ✓.bin");
    expect(baseName("/home/demo/a\\b.txt")).toBe("a\\b.txt");
  });

  it("reads the path out of an agent's refusal without doubled backslashes", () => {
    expect(outsideBasePath('path "C:\\\\Windows" is outside the allowed base directory')).toBe("C:\\Windows");
    expect(outsideBasePath('path "/etc" is outside the allowed base directory')).toBe("/etc");
    expect(outsideBasePath("folder not found")).toBeNull();
  });
});
