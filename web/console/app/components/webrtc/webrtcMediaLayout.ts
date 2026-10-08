import type { WebRTCDisplayLayout } from "./webrtcViewerTypes";

function clampVolume(value: number): number {
  if (Number.isNaN(value)) return 1;
  if (value < 0) return 0;
  if (value > 1) return 1;
  return value;
}

export function assignStreamToVideoElement(
  element: HTMLVideoElement | null,
  stream: MediaStream | null,
  audioEnabled: boolean,
  volume: number,
) {
  if (!element) {
    return;
  }
  element.srcObject = stream;
  element.muted = !audioEnabled;
  element.volume = clampVolume(volume);
}

export function normalizeDisplayLayout(
  displayLayout: WebRTCDisplayLayout[] | undefined,
): {
  displays: Array<
    WebRTCDisplayLayout & {
      leftPct: number;
      topPct: number;
      widthPct: number;
      heightPct: number;
    }
  >;
  totalWidth: number;
  totalHeight: number;
} | null {
  if (!displayLayout || displayLayout.length < 2) {
    return null;
  }

  const displays = displayLayout.filter(
    (entry) => entry.width > 0 && entry.height > 0,
  );
  if (displays.length < 2) {
    return null;
  }

  const minX = Math.min(...displays.map((entry) => entry.offset_x));
  const minY = Math.min(...displays.map((entry) => entry.offset_y));
  const maxX = Math.max(
    ...displays.map((entry) => entry.offset_x + entry.width),
  );
  const maxY = Math.max(
    ...displays.map((entry) => entry.offset_y + entry.height),
  );
  const totalWidth = maxX - minX;
  const totalHeight = maxY - minY;

  if (totalWidth <= 0 || totalHeight <= 0) {
    return null;
  }

  return {
    totalWidth,
    totalHeight,
    displays: displays.map((entry) => ({
      ...entry,
      leftPct: ((entry.offset_x - minX) / totalWidth) * 100,
      topPct: ((entry.offset_y - minY) / totalHeight) * 100,
      widthPct: (entry.width / totalWidth) * 100,
      heightPct: (entry.height / totalHeight) * 100,
    })),
  };
}
