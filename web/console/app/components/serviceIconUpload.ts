"use client";


export const MAX_CUSTOM_ICON_UPLOAD_BYTES = 512 * 1024;

export const CUSTOM_ICON_UPLOAD_TYPES = new Set([
  "image/png",
  "image/jpeg",
  "image/webp",
  "image/gif",
  "image/svg+xml",
]);

export const CUSTOM_ICON_ACCEPT = Array.from(CUSTOM_ICON_UPLOAD_TYPES).join(",");

export function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result !== "string") {
        reject(new Error("invalid file"));
        return;
      }
      resolve(reader.result);
    };
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(file);
  });
}

export function normalizeCustomIconName(filename: string): string {
  const trimmed = filename.trim();
  if (!trimmed) {
    return "Custom Icon";
  }
  const withoutExtension = trimmed.replace(/\.[^.]+$/, "");
  const normalized = withoutExtension.replace(/[_-]+/g, " ").trim();
  return normalized || "Custom Icon";
}

export function formatByteSize(size: number): string {
  if (size >= 1024 * 1024) {
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${Math.round(size / 1024)} KB`;
}
