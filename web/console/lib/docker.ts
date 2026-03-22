// Docker API client for the LabTether console.
// Calls Next.js API proxy routes which forward to the Go backend.

export interface DockerHostSummary {
  agent_id: string;
  normalized_id: string;
  engine_version: string;
  engine_os: string;
  engine_arch: string;
  container_count: number;
  image_count: number;
  stack_count: number;
  last_seen: string;
  source?: "docker" | "portainer";
}

export interface DockerContainer {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  created: string;
  ports: string;
  stack_name: string;
  labels: Record<string, string>;
  cpu_percent?: number;
  memory_percent?: number;
  memory_bytes?: number;
  memory_limit?: number;
}

export interface DockerImage {
  id: string;
  tags: string[];
  size: number;
  created: string;
}

export interface DockerNetwork {
  id: string;
  name: string;
  driver: string;
  scope: string;
}

export interface DockerVolume {
  name: string;
  driver: string;
  mountpoint: string;
}

export interface DockerStack {
  name: string;
  status: string;
  config_file: string;
  container_ids: string[];
}

export interface DockerHostDetail {
  agent_id: string;
  engine: {
    version: string;
    api_version: string;
    os: string;
    arch: string;
  };
  containers: DockerContainer[];
  images: DockerImage[];
  networks: DockerNetwork[];
  volumes: DockerVolume[];
  compose_stacks: DockerStack[];
  last_seen: string;
}

export interface DockerActionResult {
  status: string;
  message: string;
  output?: string;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asOptionalNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((entry): entry is string => typeof entry === "string");
}

function asRecordString(value: unknown): Record<string, string> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }

  const normalized: Record<string, string> = {};
  for (const [key, entry] of Object.entries(value)) {
    if (typeof entry === "string") {
      normalized[key] = entry;
    }
  }
  return normalized;
}

function normalizeDockerContainer(value: unknown): DockerContainer {
  const raw = value && typeof value === "object" ? value as Record<string, unknown> : {};
  return {
    id: asString(raw.id),
    name: asString(raw.name),
    image: asString(raw.image),
    state: asString(raw.state),
    status: asString(raw.status),
    created: asString(raw.created),
    ports: asString(raw.ports),
    stack_name: asString(raw.stack_name),
    labels: asRecordString(raw.labels),
    cpu_percent: asOptionalNumber(raw.cpu_percent),
    memory_percent: asOptionalNumber(raw.memory_percent),
    memory_bytes: asOptionalNumber(raw.memory_bytes),
    memory_limit: asOptionalNumber(raw.memory_limit),
  };
}

function normalizeDockerImage(value: unknown): DockerImage {
  const raw = value && typeof value === "object" ? value as Record<string, unknown> : {};
  return {
    id: asString(raw.id),
    tags: asStringArray(raw.tags),
    size: typeof raw.size === "number" && Number.isFinite(raw.size) ? raw.size : 0,
    created: asString(raw.created),
  };
}

function normalizeDockerStack(value: unknown): DockerStack {
  const raw = value && typeof value === "object" ? value as Record<string, unknown> : {};
  return {
    name: asString(raw.name),
    status: asString(raw.status),
    config_file: asString(raw.config_file),
    container_ids: asStringArray(raw.container_ids),
  };
}

function normalizeDockerHostSummary(value: unknown): DockerHostSummary {
  const raw = value && typeof value === "object" ? value as Record<string, unknown> : {};
  return {
    agent_id: asString(raw.agent_id),
    normalized_id: asString(raw.normalized_id),
    engine_version: asString(raw.engine_version),
    engine_os: asString(raw.engine_os),
    engine_arch: asString(raw.engine_arch),
    container_count: typeof raw.container_count === "number" && Number.isFinite(raw.container_count) ? raw.container_count : 0,
    image_count: typeof raw.image_count === "number" && Number.isFinite(raw.image_count) ? raw.image_count : 0,
    stack_count: typeof raw.stack_count === "number" && Number.isFinite(raw.stack_count) ? raw.stack_count : 0,
    last_seen: asString(raw.last_seen),
    source: (raw.source === "portainer" ? "portainer" : "docker") as "docker" | "portainer",
  };
}

function normalizeDockerHostDetail(value: unknown): DockerHostDetail {
  const raw = value && typeof value === "object" ? value as Record<string, unknown> : {};
  const engine = raw.engine && typeof raw.engine === "object" ? raw.engine as Record<string, unknown> : {};
  return {
    agent_id: asString(raw.agent_id),
    engine: {
      version: asString(engine.version),
      api_version: asString(engine.api_version),
      os: asString(engine.os),
      arch: asString(engine.arch),
    },
    containers: Array.isArray(raw.containers) ? raw.containers.map(normalizeDockerContainer) : [],
    images: Array.isArray(raw.images) ? raw.images.map(normalizeDockerImage) : [],
    networks: Array.isArray(raw.networks)
      ? raw.networks.map((entry) => {
          const network = entry && typeof entry === "object" ? entry as Record<string, unknown> : {};
          return {
            id: asString(network.id),
            name: asString(network.name),
            driver: asString(network.driver),
            scope: asString(network.scope),
          };
        })
      : [],
    volumes: Array.isArray(raw.volumes)
      ? raw.volumes.map((entry) => {
          const volume = entry && typeof entry === "object" ? entry as Record<string, unknown> : {};
          return {
            name: asString(volume.name),
            driver: asString(volume.driver),
            mountpoint: asString(volume.mountpoint),
          };
        })
      : [],
    compose_stacks: Array.isArray(raw.compose_stacks) ? raw.compose_stacks.map(normalizeDockerStack) : [],
    last_seen: asString(raw.last_seen),
  };
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, { cache: "no-store", ...init });
  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    let message = body;
    if (body) {
      try {
        const parsed = JSON.parse(body) as {
          error?: string;
          message?: string;
          result?: { message?: string };
        };
        if (typeof parsed.error === "string" && parsed.error.trim() !== "") {
          message = parsed.error;
        } else if (typeof parsed.message === "string" && parsed.message.trim() !== "") {
          message = parsed.message;
        } else if (typeof parsed.result?.message === "string" && parsed.result.message.trim() !== "") {
          message = parsed.result.message;
        }
      } catch {
        // Keep raw body fallback for non-JSON responses.
      }
    }
    throw new Error(message || `HTTP ${resp.status}`);
  }
  return resp.json();
}

export async function fetchDockerHosts(): Promise<DockerHostSummary[]> {
  const data = await fetchJSON<{ hosts: DockerHostSummary[] }>("/api/docker/hosts");
  return Array.isArray(data.hosts) ? data.hosts.map(normalizeDockerHostSummary) : [];
}

export async function fetchDockerHostDetail(hostId: string): Promise<DockerHostDetail> {
  const data = await fetchJSON<{ host?: DockerHostDetail }>(`/api/docker/hosts/${encodeURIComponent(hostId)}`);
  return normalizeDockerHostDetail(data.host);
}

export async function fetchDockerContainers(hostId: string): Promise<DockerContainer[]> {
  const data = await fetchJSON<{ containers: DockerContainer[] }>(
    `/api/docker/hosts/${encodeURIComponent(hostId)}/containers`
  );
  return Array.isArray(data.containers) ? data.containers.map(normalizeDockerContainer) : [];
}

export async function fetchDockerImages(hostId: string): Promise<DockerImage[]> {
  const data = await fetchJSON<{ images: DockerImage[] }>(
    `/api/docker/hosts/${encodeURIComponent(hostId)}/images`
  );
  return Array.isArray(data.images) ? data.images.map(normalizeDockerImage) : [];
}

export async function fetchDockerStacks(hostId: string): Promise<DockerStack[]> {
  const data = await fetchJSON<{ stacks: DockerStack[] }>(
    `/api/docker/hosts/${encodeURIComponent(hostId)}/stacks`
  );
  return Array.isArray(data.stacks) ? data.stacks.map(normalizeDockerStack) : [];
}

export async function fetchDockerContainerDetail(containerId: string): Promise<DockerContainer> {
  const data = await fetchJSON<{ container?: DockerContainer }>(`/api/docker/containers/${encodeURIComponent(containerId)}`);
  return normalizeDockerContainer(data.container);
}

export async function fetchDockerContainerStats(containerId: string): Promise<Record<string, unknown>> {
  const data = await fetchJSON<{ stats?: Record<string, unknown> }>(
    `/api/docker/containers/${encodeURIComponent(containerId)}/stats`
  );
  return data.stats ?? {};
}

export async function fetchDockerContainerLogs(
  containerId: string,
  options?: { tail?: number; timestamps?: boolean }
): Promise<string> {
  const params = new URLSearchParams();
  if (options?.tail != null) params.set("tail", String(options.tail));
  if (options?.timestamps != null) params.set("timestamps", options.timestamps ? "true" : "false");
  const query = params.toString();
  const data = await fetchJSON<{ logs?: string }>(
    `/api/docker/containers/${encodeURIComponent(containerId)}/logs${query ? `?${query}` : ""}`
  );
  return data.logs ?? "";
}

export async function executeContainerAction(
  containerId: string,
  action: string,
  params?: Record<string, string>
): Promise<DockerActionResult> {
  const data = await fetchJSON<{ result?: DockerActionResult }>(
    `/api/docker/containers/${encodeURIComponent(containerId)}/action`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ action, params: params ?? {} }),
    }
  );
  return data.result ?? { status: "failed", message: "empty docker action result" };
}

export async function executeStackAction(
  stackId: string,
  action: string,
  params?: Record<string, string>
): Promise<DockerActionResult> {
  const data = await fetchJSON<{ result?: DockerActionResult }>(
    `/api/docker/stacks/${encodeURIComponent(stackId)}/action`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ action, params: params ?? {} }),
    }
  );
  return data.result ?? { status: "failed", message: "empty stack action result" };
}

export async function executeDockerHostAction(
  hostId: string,
  action: string,
  params?: Record<string, string>,
  targetId?: string
): Promise<DockerActionResult> {
  const data = await fetchJSON<{ result?: DockerActionResult }>(
    `/api/docker/hosts/${encodeURIComponent(hostId)}/action`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        action,
        target_id: targetId ?? "",
        params: params ?? {},
      }),
    }
  );
  return data.result ?? { status: "failed", message: "empty host action result" };
}

// ── Portainer fleet integration ──

interface PortainerEndpointSummary {
  asset_id: string;
  endpoint_id: string;
  name: string;
  normalized_id: string;
  url: string;
  portainer_version: string;
  engine_os: string;
  engine_arch: string;
  container_count: number;
  stack_count: number;
  image_count: number;
  last_seen: string;
  source: string;
}

/** Fetch all Portainer endpoints as DockerHostSummary-compatible objects. */
export async function fetchPortainerEndpoints(): Promise<DockerHostSummary[]> {
  try {
    const res = await fetch("/api/portainer/endpoints", { cache: "no-store" });
    if (!res.ok) return [];
    const json = await res.json();
    const endpoints: PortainerEndpointSummary[] = json?.endpoints ?? [];
    return endpoints.map((ep): DockerHostSummary => ({
      agent_id: ep.asset_id,
      normalized_id: ep.normalized_id,
      engine_version: ep.portainer_version || "Portainer",
      engine_os: ep.engine_os || "linux",
      engine_arch: ep.engine_arch || "",
      container_count: ep.container_count,
      image_count: ep.image_count,
      stack_count: ep.stack_count,
      last_seen: ep.last_seen,
      source: "portainer",
    }));
  } catch {
    return [];
  }
}

interface PortainerRawContainer {
  Id: string;
  Names: string[];
  Image: string;
  State: string;
  Status: string;
  Ports?: Array<{ IP?: string; PrivatePort: number; PublicPort?: number; Type: string }>;
  Labels?: Record<string, string>;
  Created?: number;
}

function formatPortainerPorts(ports?: PortainerRawContainer["Ports"]): string {
  if (!ports || ports.length === 0) return "";
  return ports
    .filter((p) => p.PublicPort)
    .map((p) => `${p.PublicPort}:${p.PrivatePort}/${p.Type}`)
    .join(", ");
}

/** Fetch containers from a Portainer endpoint and normalize to DockerContainer shape. */
export async function fetchPortainerContainers(assetId: string): Promise<DockerContainer[]> {
  try {
    const res = await fetch(`/api/portainer/assets/${encodeURIComponent(assetId)}/containers`, {
      cache: "no-store",
    });
    if (!res.ok) return [];
    const json = await res.json();
    const raw: PortainerRawContainer[] = json?.data ?? [];
    return raw.map((c): DockerContainer => ({
      id: c.Id,
      name: (c.Names?.[0] ?? "").replace(/^\//, ""),
      image: c.Image,
      state: c.State?.toLowerCase() ?? "unknown",
      status: c.Status ?? "",
      created: c.Created ? new Date(c.Created * 1000).toISOString() : "",
      ports: formatPortainerPorts(c.Ports),
      stack_name: c.Labels?.["com.docker.compose.project"] ?? "",
      labels: c.Labels ?? {},
      cpu_percent: undefined,
      memory_percent: undefined,
      memory_bytes: undefined,
      memory_limit: undefined,
    }));
  } catch {
    return [];
  }
}
