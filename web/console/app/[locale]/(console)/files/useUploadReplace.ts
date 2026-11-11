"use client";

import { useCallback, useState } from "react";
import { pathStyleOf } from "./remotePaths";
import { findUploadClashes, heldEntriesFor, type FolderEntry, type HeldListing } from "./uploadClashes";

export type UploadReplaceAsk = { files: File[]; dir: string; clashing: File[]; rest: File[] };

type UploadFn = (files: File[], dir: string) => void;

type UseUploadReplaceArgs = {
  held: HeldListing;
  /** Reads a folder with its hidden files, for a target the open listing does not answer for. */
  list: (dir: string) => Promise<FolderEntry[]>;
  /** The device is a Windows one. Needed while the target is still "~"; a drive or share path says so itself. */
  ignoreCase?: boolean;
  upload: UploadFn;
  /** The target folder could not be read, so nothing was uploaded. */
  onUnread: (dir: string) => void;
};

/**
 * The step before an upload: when a chosen file has the name of a file in the target folder, nothing is uploaded
 * until the person picks Replace (all files), Skip these (only the files that do not clash) or Cancel.
 */
export function useUploadReplace({ held, list, ignoreCase = false, upload, onUnread }: UseUploadReplaceArgs) {
  const [ask, setAsk] = useState<UploadReplaceAsk | null>(null);

  /** whenClear runs instead of the upload when nothing clashes (the Files page asks before a drop of several files). */
  const start = useCallback((files: File[], dir: string, whenClear?: UploadFn) => {
    if (files.length === 0) return;
    // On Windows "Report.txt" and "report.txt" are one file, so names match in any case there.
    const anyCase = ignoreCase || pathStyleOf(dir) === "windows";
    const decide = (entries: FolderEntry[]) => {
      const found = findUploadClashes(files, entries, anyCase);
      if (found.clashing.length > 0) setAsk({ files, dir, clashing: found.clashing, rest: found.rest });
      else (whenClear ?? upload)(files, dir);
    };
    const known = heldEntriesFor(files, dir, held);
    if (known) decide(known);
    else void list(dir).then(decide, () => onUnread(dir));
  }, [held, ignoreCase, list, onUnread, upload]);

  const replace = useCallback(() => {
    if (ask) upload(ask.files, ask.dir);
    setAsk(null);
  }, [ask, upload]);

  const skip = useCallback(() => {
    if (ask && ask.rest.length > 0) upload(ask.rest, ask.dir);
    setAsk(null);
  }, [ask, upload]);

  const cancel = useCallback(() => setAsk(null), []);

  return { ask, start, replace, skip, cancel };
}
