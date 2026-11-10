import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import common from "../../../../../messages/en/common.json";
import files from "../../../../../messages/en/files.json";
import { UploadReplaceDialog } from "../FilesOverlays";
import type { FilesSourceApi } from "../useFilesSource";
import { useFilesWorkspace } from "../useFilesWorkspace";

// Risk: an upload from the Files page is written over a file of the same name without a question, and the old file
// is gone. The page must upload nothing until the person picks Replace, Skip these or Cancel.

const mocks = vi.hoisted(() => {
  const upload = vi.fn<(files: File[], dir: string) => Promise<void>>(async () => {});
  const source = {
    key: "a:nas01",
    source: { type: "agent", assetId: "nas01" },
    waiting: false,
    currentPath: "/home/demo",
    rootPath: "/",
    entries: [
      { name: "two.txt", is_dir: false, size: 4, mod_time: "", mode: "" },
      { name: "docs", is_dir: true, size: 0, mod_time: "", mode: "" },
    ],
    loading: false,
    error: null,
    showHidden: false,
    namesIgnoreCase: false,
    upload,
    listFolder: vi.fn(async () => []),
    setError: vi.fn(),
    refresh: vi.fn(),
    goTo: vi.fn(),
  };
  return { upload, source };
});

vi.mock("../useFilesSource", () => ({
  useFilesSource: () => mocks.source as unknown as FilesSourceApi,
}));

vi.mock("../useFileTransfers", () => ({
  useFileTransfers: () => ({ transfers: [], startTransfer: vi.fn(), cancelTransfer: vi.fn(), clearCompleted: vi.fn() }),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

/** The Files page's wiring of the Upload button, the file input and the Replace dialog. */
function Page({ chosen }: { chosen: File[] }) {
  const w = useFilesWorkspace();
  const { replace } = w.upload;
  return (
    <>
      <button
        type="button"
        onClick={() => {
          w.upload.pick(w.src.currentPath);
          w.upload.onInput(chosen);
        }}
      >
        Choose files
      </button>
      <UploadReplaceDialog ask={replace.ask} where="/home/demo on nas01" onCancel={replace.cancel} onSkip={replace.skip} onReplace={replace.replace} />
    </>
  );
}

const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

async function click(element: Element) {
  await act(async () => { element.dispatchEvent(new MouseEvent("click", { bubbles: true })); });
  await settle();
}

function button(label: string, scope: ParentNode = document.body): HTMLButtonElement {
  const match = [...scope.querySelectorAll<HTMLButtonElement>("button")].filter((node) => node.textContent?.trim() === label).at(-1);
  if (!match) throw new Error(`missing button: ${label}`);
  return match;
}

function dialog(): HTMLElement {
  const match = [...document.body.querySelectorAll<HTMLElement>('[role="dialog"]')].at(-1);
  if (!match) throw new Error("missing dialog");
  return match;
}

const uploadedNames = () => mocks.upload.mock.calls.map(([list, dir]) => `${dir}: ${list.map((file) => file.name).join(", ")}`);

beforeEach(() => {
  mocks.upload.mockClear();
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

it("asks before an upload replaces a file, and uploads only what the answer allows", async () => {
  const chosen = [new File(["new"], "two.txt"), new File(["new"], "notes.txt")];
  await act(async () => root.render(
    <NextIntlClientProvider locale="en" messages={{ common, files }}>
      <Page chosen={chosen} />
    </NextIntlClientProvider>,
  ));
  await settle();

  // A chosen file has the name of a file in the folder: the dialog names it and nothing is uploaded yet.
  await click(button("Choose files"));
  expect(dialog().textContent).toContain("Replace 1 file?");
  expect(dialog().textContent).toContain("two.txt is already in /home/demo on nas01");
  expect(mocks.upload).not.toHaveBeenCalled();

  // Cancel uploads nothing.
  await click(button("Cancel", dialog()));
  expect(mocks.upload).not.toHaveBeenCalled();

  // Skip these uploads only the file that does not clash.
  await click(button("Choose files"));
  await click(button("Skip these", dialog()));
  expect(uploadedNames()).toEqual(["/home/demo: notes.txt"]);

  // Replace uploads every chosen file.
  await click(button("Choose files"));
  expect(mocks.upload).toHaveBeenCalledTimes(1);
  await click(button("Replace", dialog()));
  expect(uploadedNames()).toEqual(["/home/demo: notes.txt", "/home/demo: two.txt, notes.txt"]);
});
