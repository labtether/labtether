"use client";

import { joinPath } from "./fileWorkspaceUtils";
import type {
  WorkspaceClipboard,
  WorkspaceClipboardItem,
  WorkspaceClipboardMode,
} from "./useFileWorkspaceState";

type BuildWorkspaceClipboardItemsArgs<Entry> = {
  entries: Entry[];
  names: string[];
  getName: (entry: Entry) => string;
  getPath: (entry: Entry) => string;
  getIsDir: (entry: Entry) => boolean;
};

export function buildWorkspaceClipboardItems<Entry>({
  entries,
  names,
  getName,
  getPath,
  getIsDir,
}: BuildWorkspaceClipboardItemsArgs<Entry>): WorkspaceClipboardItem[] {
  if (names.length === 0) {
    return [];
  }

  const byName = new Map(entries.map((entry) => [getName(entry), entry] as const));
  return names
    .map((name) => {
      const entry = byName.get(name);
      if (!entry) {
        return null;
      }
      return {
        name: getName(entry),
        path: getPath(entry),
        is_dir: getIsDir(entry),
      } satisfies WorkspaceClipboardItem;
    })
    .filter((item): item is WorkspaceClipboardItem => item !== null);
}

type ClipboardPathOperation = (srcPath: string, dstPath: string) => Promise<boolean>;

type ApplyWorkspaceClipboardArgs<ScopeID extends string = string> = {
  clipboard: WorkspaceClipboard<ScopeID> | null;
  ownerID: ScopeID;
  targetDirPath: string;
  copyItem: ClipboardPathOperation;
  moveItem: ClipboardPathOperation;
};

type AppliedWorkspaceClipboardResult = {
  status: "applied";
  mode: WorkspaceClipboardMode;
  total: number;
  copied: number;
  moved: number;
  skipped: number;
  failed: boolean;
};

type WorkspaceClipboardApplyResult =
  | { status: "noop" }
  | { status: "owner_mismatch" }
  | AppliedWorkspaceClipboardResult;

export async function applyWorkspaceClipboard<ScopeID extends string = string>({
  clipboard,
  ownerID,
  targetDirPath,
  copyItem,
  moveItem,
}: ApplyWorkspaceClipboardArgs<ScopeID>): Promise<WorkspaceClipboardApplyResult> {
  if (!clipboard || clipboard.items.length === 0) {
    return { status: "noop" };
  }
  if (clipboard.ownerID !== ownerID) {
    return { status: "owner_mismatch" };
  }

  let copied = 0;
  let moved = 0;
  let skipped = 0;
  let failed = false;

  for (const item of clipboard.items) {
    const destinationPath = joinPath(targetDirPath, item.name);
    if (destinationPath === item.path) {
      skipped++;
      continue;
    }

    const ok = clipboard.mode === "cut"
      ? await moveItem(item.path, destinationPath)
      : await copyItem(item.path, destinationPath);
    if (!ok) {
      failed = true;
      break;
    }
    if (clipboard.mode === "cut") {
      moved++;
    } else {
      copied++;
    }
  }

  return {
    status: "applied",
    mode: clipboard.mode,
    total: clipboard.items.length,
    copied,
    moved,
    skipped,
    failed,
  };
}
