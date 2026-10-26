import { trimTrailingChars, trimTrailingSlashes } from "../../../../lib/trimChars";

export function joinTransferDestinationPath(directory: string, sourcePath: string): string {
  const normalizedSource = trimTrailingSlashes(sourcePath.replaceAll("\\", "/"));
  const fileName = normalizedSource.split("/").pop() || "transfer";
  if (directory === "/") return `/${fileName}`;
  if (/^[A-Za-z]:[\\/]?$/.test(directory)) return `${directory.slice(0, 2)}\\${fileName}`;
  const separator = directory.includes("\\") && !directory.includes("/") ? "\\" : "/";
  const normalizedDirectory = trimTrailingChars(directory, "\\/");
  return normalizedDirectory ? `${normalizedDirectory}${separator}${fileName}` : fileName;
}
