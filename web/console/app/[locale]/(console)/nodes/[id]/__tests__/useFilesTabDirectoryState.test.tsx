import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
vi.mock("next-intl", () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) => (values ? `${key} ${Object.values(values).join(" ")}` : key),
}));

import { useFilesTabDirectoryState } from "../useFilesTabDirectoryState";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type PendingRead = { url: string; resolve: (response: Response) => void };
let pending: PendingRead[];
let container: HTMLDivElement;
let root: Root;
const setError = vi.fn();

function Consumer() {
  const state = useFilesTabDirectoryState({ nodeId: "owned-agent", setError });
  return <>
    <input aria-label="Enter path" value={state.pathInput} onChange={(event) => state.setPathInput(event.target.value)} />
    <button onClick={() => state.navigateTo(state.pathInput)}>Go</button>
    <output data-path>{state.currentPath}</output>
    <output data-loading>{String(state.loading)}</output>
    <ul>{state.entries.map((entry) => <li key={entry.name}>{entry.name}</li>)}</ul>
  </>;
}

beforeEach(() => {
  pending = [];
  setError.mockClear();
  vi.stubGlobal("fetch", vi.fn((url: string) => new Promise<Response>((resolve) => {
    pending.push({ url, resolve });
  })));
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function resolveDirectory(read: PendingRead, path: string, name: string) {
  await act(async () => {
    read.resolve(new Response(JSON.stringify({ path, entries: [{ name, is_dir: false }] })));
  });
}

it("keeps edits made during a directory read, ignores older replies, and resolves unedited navigation", async () => {
  await act(async () => root.render(<Consumer />));
  const initialReads = pending.splice(0);
  const latestInitialRead = initialReads.pop()!;
  expect(latestInitialRead).toBeDefined();
  const input = container.querySelector("input")!;
  expect(container.querySelector("[data-loading]")?.textContent).toBe("true");

  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "/home/owned/draft");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  expect(input.value).toBe("/home/owned/draft");
  await resolveDirectory(latestInitialRead, "/home/owned", "current.txt");
  expect(input.value).toBe("/home/owned/draft");
  expect(container.querySelector("[data-path]")?.textContent).toBe("/home/owned");
  expect(container.querySelector("ul")?.textContent).toBe("current.txt");
  expect(container.querySelector("[data-loading]")?.textContent).toBe("false");

  for (const stale of initialReads) {
    await resolveDirectory(stale, "/stale", "stale.txt");
  }
  expect(input.value).toBe("/home/owned/draft");
  expect(container.querySelector("[data-path]")?.textContent).toBe("/home/owned");
  expect(container.querySelector("ul")?.textContent).toBe("current.txt");

  await act(async () => container.querySelector("button")!.click());
  const navigation = pending.pop()!;
  expect(new URL(navigation.url, "http://console.test").searchParams.get("path")).toBe("/home/owned/draft");
  await resolveDirectory(navigation, "/home/owned/draft/", "next.txt");
  expect(input.value).toBe("/home/owned/draft/");
  expect(container.querySelector("[data-path]")?.textContent).toBe("/home/owned/draft/");
  expect(container.querySelector("ul")?.textContent).toBe("next.txt");
  expect(container.querySelector("[data-loading]")?.textContent).toBe("false");
  expect(setError).toHaveBeenLastCalledWith(null);
});

// Risk: a typed path the agent does not share empties the list and shows the agent's English refusal (or nothing).
it("keeps the open folder and says, in words, that a typed path is outside the shared folder", async () => {
  await act(async () => root.render(<Consumer />));
  const reads = pending.splice(0);
  for (const read of reads) await resolveDirectory(read, "/root", "notes.txt");
  const input = container.querySelector("input")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "/etc");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => { container.querySelector("button")!.dispatchEvent(new MouseEvent("click", { bubbles: true })); });
  const refused = pending.splice(0).pop()!;
  await act(async () => {
    refused.resolve(new Response(JSON.stringify({ error: 'path "/etc" is outside the allowed base directory' }), { status: 400 }));
  });
  expect(setError).toHaveBeenLastCalledWith("outsideShared /etc");
  expect(container.querySelector("[data-path]")?.textContent).toBe("/root");
  expect([...container.querySelectorAll("li")].map((li) => li.textContent)).toEqual(["notes.txt"]);
  expect(input.value).toBe("/root");
});
