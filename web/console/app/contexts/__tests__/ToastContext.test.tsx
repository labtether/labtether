import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ToastProvider, useToast } from "../ToastContext";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function ToastHarness() {
  const { addToast } = useToast();
  return (
    <>
      <button type="button" onClick={() => addToast("error", "Connection failed.", 0)}>Error</button>
      <button type="button" onClick={() => addToast("success", "Connection restored.", 0)}>Success</button>
    </>
  );
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("ToastProvider accessibility", () => {
  it("announces failures assertively and success messages politely", async () => {
    await act(async () => {
      root.render(<ToastProvider><ToastHarness /></ToastProvider>);
    });

    const [errorButton, successButton] = [...container.querySelectorAll("button")];
    await act(async () => errorButton.click());
    await act(async () => successButton.click());

    const errorToast = container.querySelector<HTMLElement>(".toast-error");
    const successToast = container.querySelector<HTMLElement>(".toast-success");
    expect(errorToast?.getAttribute("role")).toBe("alert");
    expect(errorToast?.getAttribute("aria-live")).toBe("assertive");
    expect(successToast?.getAttribute("role")).toBe("status");
    expect(successToast?.getAttribute("aria-live")).toBe("polite");
  });
});
