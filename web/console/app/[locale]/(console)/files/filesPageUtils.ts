import type { LucideIcon } from "lucide-react";
import {
  Folder, FileText, FileCode, Image, Film, Music, Archive,
  FileKey, Settings, File, FileSpreadsheet, Terminal, BookOpen,
} from "lucide-react";

export function formatSize(bytes: number): string {
  if (bytes === 0) return "\u2014";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return `${val.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

export function formatDate(iso: string): string {
  if (!iso) return "\u2014";
  const date = new Date(iso);
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function fileIconComponent(name: string, isDir: boolean): LucideIcon {
  if (isDir) return Folder;
  const ext = name.includes(".") ? name.split(".").pop()?.toLowerCase() : "";
  switch (ext) {
    case "zip": case "tar": case "gz": case "bz2": case "xz": case "7z": case "rar":
      return Archive;
    case "png": case "jpg": case "jpeg": case "gif": case "svg": case "webp": case "ico": case "bmp":
      return Image;
    case "mp4": case "mkv": case "avi": case "mov": case "webm":
      return Film;
    case "mp3": case "flac": case "wav": case "ogg": case "aac":
      return Music;
    case "go": case "ts": case "tsx": case "js": case "jsx": case "py": case "rs": case "c": case "cpp": case "h": case "java": case "rb": case "swift":
      return FileCode;
    case "json": case "yaml": case "yml": case "toml": case "xml": case "ini": case "conf": case "cfg": case "env":
      return Settings;
    case "md": case "txt": case "log":
      return FileText;
    case "csv":
      return FileSpreadsheet;
    case "sh": case "bash": case "zsh": case "fish": case "bat": case "ps1":
      return Terminal;
    case "pdf":
      return BookOpen;
    case "key": case "pem": case "crt": case "cert":
      return FileKey;
    default:
      return File;
  }
}
