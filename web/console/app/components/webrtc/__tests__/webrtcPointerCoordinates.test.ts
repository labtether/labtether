import { describe, expect, it } from "vitest";
import { remotePointerCoordinates } from "../webrtcPointerCoordinates";

const rect = { left: 100, top: 50, width: 800, height: 600 };

describe("remote video pointer coordinates", () => {
  it("accounts for letterboxing and uses video pixels at every scale", () => {
    expect(remotePointerCoordinates(300, 237.5, rect, 1920, 1080, "fit")).toEqual({ x: 480, y: 270 });
    expect(remotePointerCoordinates(100, 50, rect, 1920, 1080, "fit")).toEqual({ x: 0, y: 0 });
    expect(remotePointerCoordinates(900, 650, rect, 1920, 1080, "fit")).toEqual({ x: 1919, y: 1079 });
  });

  it("accounts for cropped cover mode and stretched monitor layouts", () => {
    expect(remotePointerCoordinates(100, 50, rect, 1920, 1080, "fill")).toEqual({ x: 240, y: 0 });
    expect(remotePointerCoordinates(500, 350, rect, 3840, 1080, "stretch")).toEqual({ x: 1920, y: 540 });
  });

  it("does not emit coordinates before the video size is known", () => {
    expect(remotePointerCoordinates(100, 50, rect, 0, 0, "fit")).toBeNull();
  });
});
