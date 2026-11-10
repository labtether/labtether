// An upload is written over a file of the same name in its target folder, so the file views ask before one starts.
// Shared by the Files page, a device's Files tab and the Remote View files drawer.

type Named = { name: string };

export type FolderEntry = { name: string; is_dir: boolean };

/** The folder a file view has open. ready: the entries are the folder's current listing (not loading, no error). */
export type HeldListing = { path: string; entries: FolderEntry[]; showHidden: boolean; ready: boolean };

export type UploadClashes<F extends Named> = { clashing: F[]; rest: F[] };

/**
 * Splits the chosen files into those a file in the folder already has the name of, and the rest. A folder of the
 * same name does not count: an upload cannot replace it and fails instead. ignoreCase is for Windows devices.
 */
export function findUploadClashes<F extends Named>(files: F[], entries: FolderEntry[], ignoreCase = false): UploadClashes<F> {
  const key = (name: string) => (ignoreCase ? name.toLowerCase() : name);
  const taken = new Set(entries.filter((entry) => !entry.is_dir).map((entry) => key(entry.name)));
  const clashing: F[] = [];
  const rest: F[] = [];
  for (const file of files) (taken.has(key(file.name)) ? clashing : rest).push(file);
  return { clashing, rest };
}

/**
 * The open listing's entries when they answer for these names in this folder. Null when the folder has to be read
 * first: it is another folder, the listing is not ready, or a dot file was chosen while hidden files are not listed.
 */
export function heldEntriesFor(files: Named[], dir: string, held: HeldListing): FolderEntry[] | null {
  if (!held.ready || dir !== held.path) return null;
  if (!held.showHidden && files.some((file) => file.name.startsWith("."))) return null;
  return held.entries;
}
