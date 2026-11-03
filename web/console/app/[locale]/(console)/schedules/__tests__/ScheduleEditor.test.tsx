import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Risk: picking Custom after "Every day at 02:00" turns the line into UTC cron, so the schedule runs hours off.

vi.mock("next-intl", () => ({
  useLocale: () => "en",
  useTranslations: () => (key: string) => key,
}));

vi.mock("../cronModel", async (original) => ({
  ...(await original<typeof import("../cronModel")>()),
  browserZone: () => "Australia/Sydney",
}));

import { ScheduleEditor } from "../ScheduleEditor";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

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

describe("ScheduleEditor timing", () => {
  it("starts Custom from the preset's line in the browser's zone", async () => {
    await act(async () => root.render(<ScheduleEditor devices={[]} groups={[]} onSave={async () => undefined} onClose={() => undefined} />));
    const when = container.querySelector<HTMLSelectElement>("#sf-when")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set?.call(when, "custom");
      when.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(container.querySelector<HTMLInputElement>("#sf-cron")?.value).toBe("CRON_TZ=Australia/Sydney 0 2 * * *");
  });
});
