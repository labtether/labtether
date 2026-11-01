import { describe, expect, it } from "vitest";
import type { DockerContainer, DockerStack } from "../../../../lib/docker";
import { iconKeyFor } from "../containerIcons";
import {
  buildSections,
  dockerContainerAssetId,
  filterRows,
  hostKey,
  parseDockerStatus,
  parsePorts,
  portainerContainerAssetId,
  sortRows,
  toRow,
  type CtHost,
} from "../containerModel";
import { runErrors, runParams, stackErrors } from "../deployModel";
import { imageRows, stackMembers, stackRoot, stackState } from "../stackModel";

const ct = (over: Partial<DockerContainer>): DockerContainer => ({
  id: "0123456789abcdef", name: "app", image: "nginx:1.29", state: "running", status: "Up 2 hours",
  created: "", ports: "", stack_name: "", labels: {}, ...over,
});
const host = (key: string): CtHost => ({ key, label: key, source: "docker", href: `/nodes/docker-host-${key}`, canDeploy: true });

// The state word and dot on every row come from Docker's own status line.
describe("parseDockerStatus", () => {
  it("reads when a container exited, without the exit code", () => {
    expect(parseDockerStatus("exited", "Exited (137) 2 days ago")).toMatchObject({ kind: "exited", ago: { n: 2, unit: "day" } });
    expect(parseDockerStatus("exited", "Exited (0) About an hour ago").ago).toEqual({ n: 1, unit: "hour" });
  });
  it("reads uptime, health and a paused container from the status line alone", () => {
    expect(parseDockerStatus("running", "Up 6 days (unhealthy)")).toMatchObject({ kind: "running", up: { n: 6, unit: "day" }, health: "unhealthy" });
    expect(parseDockerStatus("", "Up 3 hours (Paused)").kind).toBe("paused");
    expect(parseDockerStatus("running", "Up Less than a second").up).toEqual({ n: 0, unit: "second" });
  });
});

// Published ports: Docker lists IPv4 and IPv6 bindings separately and Portainer uses host:container.
describe("parsePorts", () => {
  it("keeps each published port once and drops unpublished ones", () => {
    expect(parsePorts("8096->8096/tcp, 8096->8096/tcp, 1900->1900/udp, 0->9000/tcp")).toEqual([
      { host: 1900, inside: 1900, proto: "udp" },
      { host: 8096, inside: 8096, proto: "tcp" },
    ]);
    expect(parsePorts("8080:80/tcp")).toEqual([{ host: 8080, inside: 80, proto: "tcp" }]);
    expect(parsePorts("0.0.0.0:8096->8096/tcp, :::8096->8096/tcp")).toEqual([{ host: 8096, inside: 8096, proto: "tcp" }]);
  });
});

// Links must match the hub's asset ids, or Open and the row actions reach no container.
describe("asset ids", () => {
  it("keeps a host named docker-01 intact", () => {
    expect(hostKey("docker-host-docker-01")).toBe("docker-01");
    expect(dockerContainerAssetId("docker-01", "0123456789abcdef")).toBe("docker-ct-docker-01-0123456789ab");
    expect(hostKey("NAS.Local")).toBe("nas-local");
  });
  it("names Portainer containers by endpoint number", () => {
    expect(portainerContainerAssetId("portainer-endpoint-3", "0123456789abcdef")).toBe("portainer-container-3-0123456789ab");
    expect(portainerContainerAssetId("odd", "0123")).toBe("");
  });
});

describe("list rows", () => {
  const h1 = host("docker-01");
  const h2 = host("nuc-01");
  const rows = [
    toRow(h1, ct({ id: "a1", name: "grafana", stack_name: "monitoring", cpu_percent: 0.3, memory_bytes: 140e6 })),
    toRow(h1, ct({ id: "a2", name: "jellyfin", cpu_percent: 4.1, memory_bytes: 1.2e9 })),
    toRow(h1, ct({ id: "a3", name: "loki", stack_name: "monitoring", cpu_percent: 0.2 })),
    toRow(h1, ct({ id: "a4", name: "prometheus", stack_name: "monitoring", cpu_percent: 1.1 })),
    toRow(h1, ct({ id: "a5", name: "watchtower", state: "exited", status: "Exited (0) 2 days ago", cpu_percent: 9 })),
    toRow(h2, ct({ id: "b1", name: "mosquitto", image: "eclipse-mosquitto:2.0.22" })),
  ];

  it("sorts by CPU with stopped containers last, whichever way", () => {
    expect(sortRows(rows, { key: "cpu", dir: "desc" }).map((r) => r.name)).toEqual(["jellyfin", "prometheus", "grafana", "loki", "mosquitto", "watchtower"]);
    expect(sortRows(rows, { key: "cpu", dir: "asc" }).at(-1)?.name).toBe("watchtower");
  });

  it("filters by host name and by state", () => {
    expect(filterRows(rows, "nuc", "all").map((r) => r.name)).toEqual(["mosquitto"]);
    expect(filterRows(rows, "", "stopped").map((r) => r.name)).toEqual(["watchtower"]);
  });

  it("keeps a stack together under its host on one rail, naming the stack once", () => {
    const [first] = buildSections(sortRows(rows, { key: "name", dir: "asc" }), "host", [h1, h2]);
    expect(first.rows.map((r) => [r.row.name, r.rail, r.showStack])).toEqual([
      ["grafana", "first", true], ["loki", "mid", false], ["prometheus", "last", false], ["jellyfin", "", true], ["watchtower", "", true],
    ]);
  });

  it("groups by stack A–Z with containers outside a stack last", () => {
    expect(buildSections(rows, "stack", [h1, h2]).map((s) => [s.kind, s.rows.length])).toEqual([["stack", 3], ["nostack", 3]]);
  });
});

describe("stacks and images", () => {
  const stack: DockerStack = { name: "Immich", status: "running(2)", config_file: "/opt/stacks/immich/compose.yaml", container_ids: null as unknown as string[] };
  const members = [ct({ id: "c1", name: "immich-server", stack_name: "immich" }), ct({ id: "c2", name: "immich-db", stack_name: "immich", state: "exited", status: "Exited (0) 1 hour ago" })];

  it("finds members by compose project even when the agent sends no id list", () => {
    expect(stackMembers(stack, [...members, ct({ id: "c3", name: "other" })]).map((c) => c.name)).toEqual(["immich-server", "immich-db"]);
    expect(stackState(members)).toMatchObject({ running: 1, total: 2, word: "partial", tone: "warn" });
  });

  it("names the shared stacks folder only when every stack lives in it", () => {
    const other: DockerStack = { ...stack, name: "media", config_file: "/opt/stacks/media/compose.yaml" };
    expect(stackRoot([stack, other])).toBe("/opt/stacks");
    expect(stackRoot([stack, { ...other, config_file: "/srv/media/compose.yaml" }])).toBe("");
  });

  it("matches an image to the containers that run it, a bare name meaning :latest", () => {
    const [row] = imageRows([{ id: "sha256:abc", tags: ["redis:latest"], size: 1, created: "" }], [ct({ name: "cache", image: "redis" }), ct({ name: "web" })]);
    expect(row.users).toEqual(["cache"]);
  });
});

// The agent silently skips port entries that are not host:container, so the form names them first.
describe("deploy checks", () => {
  const base = { image: "nginx:1.29", name: "", ports: "", env: "", command: "" };
  it("accepts a normal run and sends ports comma-joined", () => {
    expect(runErrors({ ...base, ports: "8080:80\n5353:53/udp" })).toEqual({});
    expect(runParams({ ...base, ports: "8080:80\n5353:53/udp", env: "TZ=Etc/UTC\n\nPUID=1000" })).toMatchObject({ ports: "8080:80,5353:53/udp", env: "TZ=Etc/UTC\nPUID=1000" });
  });
  it("names what the agent would skip or refuse", () => {
    expect(runErrors({ ...base, image: "", ports: "10.20.0.31:8080:80", env: "TZ" })).toEqual({ image: "required", ports: "ports", env: "env" });
    expect(runErrors({ ...base, ports: "70000:80" }).ports).toBe("portRange");
    expect(stackErrors({ name: "Media", compose: "version: '3'" })).toEqual({ name: "stackName", compose: "services" });
  });
});

describe("iconKeyFor", () => {
  const index = new Set(["vaultwarden", "immich", "plex", "jellyfin"]);
  it("finds the app behind registry paths, generic repo names and part names", () => {
    expect(iconKeyFor("vaultwarden/server:1.34.3", "vaultwarden", index)).toBe("vaultwarden");
    expect(iconKeyFor("ghcr.io/immich-app/immich-server:v1.140.1", "immich-server", index)).toBe("immich");
    expect(iconKeyFor("plexinc/pms-docker:1.42.1", "plex", index)).toBe("plex");
    expect(iconKeyFor("lscr.io/linuxserver/jellyfin:10.10.7", "media", index)).toBe("jellyfin");
    expect(iconKeyFor("ghcr.io/example/thing:1", "thing", index)).toBeNull();
  });
});
