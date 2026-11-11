import { NextIntlClientProvider } from "next-intl";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import devices from "../../../../../../messages/en/devices.json";
import type { Asset } from "../../../../../console/models";
import { DeviceEditCard } from "../DeviceEditCard";
import { deviceEditPatch, draftOf, parseTags } from "../deviceEdit";

// Risk: a device added by hand cannot be renamed, regrouped or tagged in the console, or a save sends fields the
// person did not change (a group or tags wiped by a rename).
const manual = {
  id: "manual_1000000000000000001_1", name: "nas01", type: "device", source: "manual", status: "online",
  last_seen_at: "2026-11-10T11:45:00Z", group_id: "grp-1", tags: ["backup"],
} as Asset;

describe("deviceEditPatch", () => {
  it("sends only what changed", () => {
    expect(deviceEditPatch(manual, { ...draftOf(manual), name: "nas02" })).toEqual({ name: "nas02" });
    expect(deviceEditPatch(manual, { ...draftOf(manual), groupId: "" })).toEqual({ group_id: "" });
    expect(deviceEditPatch(manual, { ...draftOf(manual), tags: "backup, media ,backup" })).toEqual({ tags: ["backup", "media"] });
    expect(deviceEditPatch(manual, { ...draftOf(manual), tags: "" })).toEqual({ tags: [] });
    expect(deviceEditPatch(manual, { ...draftOf(manual), name: "  nas01 " })).toBeNull();
    expect(deviceEditPatch(manual, { ...draftOf(manual), name: " " })).toBe("nameRequired");
    expect(parseTags(" a, ,b,a ")).toEqual(["a", "b"]);
  });
});

describe("DeviceEditCard", () => {
  let root: Root;
  let container: HTMLDivElement;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  const type = (input: HTMLInputElement, value: string) => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  };

  // Seen in the browser: the status refresh from one save landed while the next change was being typed and
  // replaced it with the Hub's values.
  it("keeps what the person is typing when a refresh arrives, and takes the Hub's values otherwise", async () => {
    const show = async (asset: Asset) => {
      await act(async () => {
        root.render(
          <NextIntlClientProvider locale="en" messages={{ devices }}>
            <DeviceEditCard asset={asset} groups={[]} onSaved={() => {}} />
          </NextIntlClientProvider>,
        );
      });
    };
    const tags = () => container.querySelector<HTMLInputElement>("#dst-dev-tags")!;
    await show(manual);
    await act(async () => type(tags(), "backup, media"));
    await show({ ...manual, name: "nas02" });
    // The field being typed keeps its text; the name, untouched, follows the Hub.
    expect(tags().value).toBe("backup, media");
    expect(container.querySelector<HTMLInputElement>("#dst-dev-name")!.value).toBe("nas02");

    await act(async () => type(tags(), "backup"));
    await show({ ...manual, name: "nas03", tags: ["backup", "nas"] });
    expect(container.querySelector<HTMLInputElement>("#dst-dev-name")!.value).toBe("nas03");
    expect(tags().value).toBe("backup, nas");
  });

  it("renames a device added by hand and keeps its group and tags", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ ...manual, name: "nas02" }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const onSaved = vi.fn();
    await act(async () => {
      root.render(
        <NextIntlClientProvider locale="en" messages={{ devices }}>
          <DeviceEditCard asset={manual} groups={[{ id: "grp-1", name: "Rack 1" } as never]} onSaved={onSaved} />
        </NextIntlClientProvider>,
      );
    });
    expect(container.querySelector("h2")?.textContent).toBe("Device");
    await act(async () => type(container.querySelector<HTMLInputElement>("#dst-dev-name")!, "nas02"));
    await act(async () => container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));

    expect(fetchMock).toHaveBeenCalledWith(`/api/assets/${manual.id}`, expect.objectContaining({ method: "PATCH", body: JSON.stringify({ name: "nas02" }) }));
    expect(onSaved).toHaveBeenCalledOnce();
    expect(container.querySelector("[role=status]")?.textContent).toBe("Saved.");
  });
});
