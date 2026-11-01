// Checks for Run container and Deploy stack, matching what the Docker agent accepts, so a bad value is named on the
// form instead of being skipped or rejected on the host.

export type RunValues = { image: string; name: string; ports: string; env: string; command: string };
export type RunErrors = Partial<Record<"image" | "name" | "ports" | "env", "image" | "name" | "ports" | "portRange" | "env" | "required">>;
export type StackValues = { name: string; compose: string };
export type StackErrors = Partial<Record<"name" | "compose", "stackName" | "services" | "required">>;

const IMAGE = /^[a-z0-9][\w.-]*(:\d+)?(\/[\w.-]+)*(:[\w.-]+)?(@sha256:[a-f0-9]{64})?$/i;
/** Docker's container name rule. */
const NAME = /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/;
/** host:container, with an optional protocol: the agent skips anything else. */
const PORT = /^(\d{1,5}):(\d{1,5})(\/(tcp|udp|sctp))?$/i;
const ENV = /^[A-Za-z_][A-Za-z0-9_]*=/;
/** Compose's project name rule. */
const STACK = /^[a-z0-9][a-z0-9_-]*$/;

/** One entry per line or comma, blanks dropped. */
export const splitList = (raw: string) => raw.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
const lines = (raw: string) => raw.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);

/** "lscr.io/linuxserver/jellyfin:10.10.7" → "jellyfin": the name Docker would show if none is given. */
export const repoName = (image: string) => (image.trim().replace(/@sha256:.*$/, "").replace(/:[^:/]+$/, "").split("/").pop() ?? "");

export function runErrors(v: RunValues): RunErrors {
  const err: RunErrors = {};
  const image = v.image.trim();
  if (!image) err.image = "required";
  else if (!IMAGE.test(image)) err.image = "image";
  if (v.name.trim() && !NAME.test(v.name.trim())) err.name = "name";
  const ports = splitList(v.ports);
  const bad = ports.find((p) => !PORT.test(p));
  if (bad) err.ports = "ports";
  else if (ports.some((p) => { const m = PORT.exec(p)!; return [m[1], m[2]].some((n) => Number(n) < 1 || Number(n) > 65535); })) err.ports = "portRange";
  if (lines(v.env).some((l) => !ENV.test(l))) err.env = "env";
  return err;
}

/** The hub's container.create parameters: ports comma-joined, environment one per line. */
export function runParams(v: RunValues): Record<string, string> {
  return {
    image: v.image.trim(),
    name: v.name.trim(),
    command: v.command.trim(),
    env: lines(v.env).join("\n"),
    ports: splitList(v.ports).join(","),
  };
}

export function stackErrors(v: StackValues): StackErrors {
  const err: StackErrors = {};
  const name = v.name.trim();
  if (!name) err.name = "required";
  else if (!STACK.test(name)) err.name = "stackName";
  if (!v.compose.trim()) err.compose = "required";
  else if (!/^services:\s*(#.*)?$/m.test(v.compose)) err.compose = "services";
  return err;
}

export const COMPOSE_SAMPLE = `services:
  whoami:
    image: traefik/whoami:v1.11.0
    restart: unless-stopped
    ports:
      - "8081:80"
`;
