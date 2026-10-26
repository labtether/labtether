type Rect = { left: number; top: number; width: number; height: number };

// object-fit changes the image bounds inside the video element, including
// letterboxing (contain) and cropped image edges (cover).
export function remotePointerCoordinates(
  clientX: number,
  clientY: number,
  rect: Rect,
  width: number,
  height: number,
  fit: "fit" | "fill" | "native" | "stretch",
): { x: number; y: number } | null {
  if (width <= 0 || height <= 0 || rect.width <= 0 || rect.height <= 0) return null;
  let renderedWidth = rect.width;
  let renderedHeight = rect.height;
  if (fit !== "stretch") {
    const scale = fit === "native" ? 1 : fit === "fill"
      ? Math.max(rect.width / width, rect.height / height)
      : Math.min(rect.width / width, rect.height / height);
    renderedWidth = width * scale;
    renderedHeight = height * scale;
  }
  const left = rect.left + (rect.width - renderedWidth) / 2;
  const top = rect.top + (rect.height - renderedHeight) / 2;
  return {
    x: Math.max(0, Math.min(width - 1, Math.round((clientX - left) * width / renderedWidth))),
    y: Math.max(0, Math.min(height - 1, Math.round((clientY - top) * height / renderedHeight))),
  };
}
