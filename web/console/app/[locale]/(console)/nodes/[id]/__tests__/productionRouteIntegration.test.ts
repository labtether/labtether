import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

function readNodeSource(path: string): string {
  return readFileSync(join(process.cwd(), "app/[locale]/(console)/nodes/[id]", path), "utf8");
}

describe("node production route contracts", () => {
  it("uses POST for every Portainer stack action supported by the hub", () => {
    const source = readNodeSource("portainer/PortainerStacksTab.tsx");

    expect(source).toContain('"POST",');
    expect(source).not.toContain('action === "remove" ? "DELETE"');
  });

  it("opens Proxmox consoles through the authenticated desktop bridge", () => {
    const source = readNodeSource("proxmox/ProxmoxConsoleTab.tsx");

    expect(source).toContain("?panel=desktop");
    expect(source).not.toContain("/vncproxy");
    expect(source).not.toContain("vncproxyUrl");
  });

  it("renders Portainer overview metadata using the hub response keys", () => {
    const source = readNodeSource("portainer/PortainerOverviewTab.tsx");

    expect(source).toContain("data.server_version");
    expect(source).toContain("data.endpoint_name");
    expect(source).toContain("data.endpoint_url");
  });
});
