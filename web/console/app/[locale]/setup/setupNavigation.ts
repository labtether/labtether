import { safeLocalRedirectPath } from "../../../lib/safeRedirect";

export function buildLocalSetupDestination(nextPath: string): string {
  return safeLocalRedirectPath(nextPath);
}

export function buildRemoteAccessURL(baseURL: string, nextPath: string): string {
  try {
    const target = new URL(baseURL);
    target.pathname = buildLocalSetupDestination(nextPath);
    target.search = "";
    target.hash = "";
    return target.toString();
  } catch {
    return baseURL;
  }
}
