export function joinTransferDestinationPath(directory: string, sourcePath: string): string {
  const normalizedSource = sourcePath.replaceAll("\\", "/").replace(/\/+$/, "");
  const fileName = normalizedSource.split("/").pop() || "transfer";
  if (directory === "/") return `/${fileName}`;
  if (/^[A-Za-z]:[\\/]?$/.test(directory)) return `${directory.slice(0, 2)}\\${fileName}`;
  const separator = directory.includes("\\") && !directory.includes("/") ? "\\" : "/";
  const normalizedDirectory = directory.replace(/[\\/]+$/, "");
  return normalizedDirectory ? `${normalizedDirectory}${separator}${fileName}` : fileName;
}
