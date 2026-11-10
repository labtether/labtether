import { baseName, joinPath } from "./remotePaths";

/** Where a transferred file lands: the destination folder plus the source file's own name. */
export function joinTransferDestinationPath(directory: string, sourcePath: string): string {
  const fileName = baseName(sourcePath) || "transfer";
  return directory ? joinPath(directory, fileName) : fileName;
}
