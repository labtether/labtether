import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: on a device that shares only its home folder, the parts of the path above it ("/" and "home" on Linux, "C:"
// and "Users" on Windows) were buttons that could only answer "… is outside the folder this device shares".
vi.mock("next-intl", () => ({ useTranslations: () => (key: string) => key }));

import { FilesPathBar } from "../FilesPathBar";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: Root;

/** Each part of the path as the bar draws it: "button home" can be opened, "text home" cannot. */
async function crumbs(path: string, shared?: string): Promise<string[]> {
  await act(async () => root.render(<FilesPathBar path={path} root={shared} onGo={() => {}} canUp onUp={() => {}} />));
  return [...host.querySelectorAll(".fx-crumbs > :not(.fx-sep)")]
    .map((part) => `${part.tagName === "BUTTON" ? "button" : "text"} ${part.textContent}`);
}

beforeEach(() => {
  host = document.createElement("div");
  root = createRoot(host);
});

afterEach(() => act(() => root.unmount()));

it("draws the folders above the shared folder as text and the rest as buttons", async () => {
  expect(await crumbs("/home/demo/docs/2026", "/home/demo")).toEqual(["text /", "text home", "button demo", "button docs", "text 2026"]);
  expect(await crumbs("C:\\Users\\demo\\docs", "C:\\Users\\demo")).toEqual(["text C:", "text Users", "button demo", "text docs"]);
});

it("keeps every folder a button on a device that shares the whole file system, and before the shared folder is known", async () => {
  expect(await crumbs("/home/demo/docs", "/")).toEqual(["button /", "button home", "button demo", "text docs"]);
  expect(await crumbs("C:\\Users\\demo", "C:\\")).toEqual(["button C:", "button Users", "text demo"]);
  expect(await crumbs("/home/demo/docs", "~")).toEqual(["button /", "button home", "button demo", "text docs"]);
  expect(await crumbs("/srv/share", undefined)).toEqual(["button /", "button srv", "text share"]);
});
