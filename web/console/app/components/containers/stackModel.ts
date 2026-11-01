// Compose stacks and images on a Docker host: which containers belong to each stack, how many run, where the compose
// files live, and which containers use each image.
import type { DockerContainer, DockerImage, DockerStack } from "../../../lib/docker";
import { parseDockerStatus, type Tone } from "./containerModel";

/** A stack's containers: by compose project label, or by the names and ids the agent lists for it. */
export function stackMembers(stack: DockerStack, containers: DockerContainer[]): DockerContainer[] {
  const name = stack.name.toLowerCase();
  const linked = new Set((stack.container_ids ?? []).map((v) => v.toLowerCase()));
  return containers.filter((c) => {
    if ((c.stack_name ?? "").toLowerCase() === name) return true;
    const id = c.id.toLowerCase();
    return linked.has(c.name.toLowerCase()) || linked.has(id) || linked.has(id.slice(0, 12));
  });
}

export type StackState = { running: number; total: number; tone: Tone; word: "" | "partial" | "stopped" | "busy" | "empty" };

/** All running: no word. Some: "2 of 3 running" (warn). None: Stopped. Any restarting: busy. */
export function stackState(members: DockerContainer[], status = ""): StackState {
  const states = members.map((c) => parseDockerStatus(c.state, c.status));
  const running = states.filter((s) => s.kind === "running").length;
  const total = members.length;
  if (states.some((s) => s.kind === "restarting" || s.kind === "removing")) return { running, total, tone: "busy", word: "busy" };
  if (!total) return /running/i.test(status) ? { running, total, tone: "", word: "" } : { running, total, tone: "off", word: "empty" };
  if (running === total) return { running, total, tone: "", word: "" };
  return running ? { running, total, tone: "warn", word: "partial" } : { running, total, tone: "off", word: "stopped" };
}

const dirOf = (path: string) => path.replace(/\/[^/]*$/, "");

/** The folder holding a stack's compose file, or "". */
export const stackFolder = (stack: DockerStack) => (stack.config_file ? dirOf(stack.config_file) : "");

/** The folder every stack's folder sits in ("/opt/stacks"), or "" when they do not share one. */
export function stackRoot(stacks: DockerStack[]): string {
  const parents = [...new Set(stacks.map(stackFolder).filter(Boolean).map(dirOf))];
  return parents.length === 1 && parents[0] ? parents[0] : "";
}

export type ImageRow = { id: string; tags: string[]; size: number; created: string; users: string[] };

/** Which containers run each image: by tag (a bare name means :latest), or by image id when a container names it by id. */
export function imageRows(images: DockerImage[], containers: DockerContainer[]): ImageRow[] {
  return images.map((img) => {
    const id = img.id.replace(/^sha256:/, "");
    const users = containers
      .filter((c) => {
        if (img.tags.includes(c.image) || (!c.image.includes(":") && img.tags.includes(`${c.image}:latest`))) return true;
        const ref = c.image.replace(/^sha256:/, "");
        return !!id && /^[0-9a-f]{12,}$/.test(ref) && id.startsWith(ref.slice(0, 12));
      })
      .map((c) => c.name);
    return { id, tags: img.tags, size: img.size, created: img.created, users };
  });
}
