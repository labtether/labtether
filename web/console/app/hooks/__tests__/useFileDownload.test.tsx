import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ download: vi.fn(), save: vi.fn() }));
vi.mock("../../[locale]/(console)/files/fileOpsClient", () => ({ downloadFileResponse: mocks.download }));
vi.mock("../../[locale]/(console)/files/fileWorkspaceUtils", () => ({ triggerBlobDownload: mocks.save }));
import { useFileDownload } from "../useFileDownload";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let hook: ReturnType<typeof useFileDownload>;
function Harness({ nodeId = "node-a" }: { nodeId?: string }) { hook = useFileDownload(nodeId); return null; }
function response(blob = new Blob(["abc"])) {
  return { ok: true, headers: new Headers(), body: null, blob: async () => blob } as Response;
}

beforeEach(async () => {
  vi.resetAllMocks();
  root = createRoot(document.createElement("div"));
  await act(async () => { root.render(<Harness />); });
});

afterEach(async () => { await act(async () => { root.unmount(); }); });

describe("remote file download ownership", () => {
  it("uses the basename of Windows paths", async () => {
    const file = new Blob(["abc"]);
    mocks.download.mockResolvedValue(response(file));
    await act(async () => { await hook.downloadFile("C:\\Users\\Me\\report.txt"); });
    expect(mocks.save).toHaveBeenCalledWith(file, "report.txt");
  });

  it("keeps cancel attached to the latest download when an old request finishes", async () => {
    const signals: AbortSignal[] = [];
    const finishes: Array<(res: Response) => void> = [];
    mocks.download.mockImplementation((_node, _path, options) => {
      signals.push(options.signal);
      return new Promise<Response>((resolve) => { finishes.push(resolve); });
    });
    let first!: Promise<void>;
    let second!: Promise<void>;
    await act(async () => { first = hook.downloadFile("/old.txt"); });
    await act(async () => { second = hook.downloadFile("/new.txt"); });
    expect(signals[0].aborted).toBe(true);
    await act(async () => { finishes[0](response()); await first; });
    expect(hook.filename).toBe("new.txt");
    expect(hook.downloading).toBe(true);
    await act(async () => { hook.cancel(); finishes[1](response()); await second; });
    expect(signals[1].aborted).toBe(true);
    expect(mocks.save).not.toHaveBeenCalled();
    expect(hook.downloading).toBe(false);
  });

  it("does not save a previous node's late response", async () => {
    let finish!: (res: Response) => void;
    mocks.download.mockReturnValue(new Promise<Response>((resolve) => { finish = resolve; }));
    let pending!: Promise<void>;
    await act(async () => { pending = hook.downloadFile("/private.txt"); });
    await act(async () => { root.render(<Harness nodeId="node-b" />); });
    await act(async () => { finish(response()); await pending; });
    expect(mocks.save).not.toHaveBeenCalled();
    expect(hook.filename).toBeNull();
  });
});
