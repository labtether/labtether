const LOCAL_BASE = "https://labtether.invalid";
const forbiddenEncodedSeparators = /%(?:2f|5c)/i;
const forbiddenCharacters = /[\\\u0000-\u001f\u007f]/;

/** Canonicalize a redirect and require it to stay on the local origin. */
export function safeLocalRedirectPath(value: unknown, fallback = "/"): string {
  if (typeof value !== "string") return fallback;
  const candidate = value.trim();
  if (
    candidate === "" ||
    !candidate.startsWith("/") ||
    candidate.startsWith("//") ||
    forbiddenCharacters.test(candidate) ||
    forbiddenEncodedSeparators.test(candidate)
  ) {
    return fallback;
  }

  try {
    const parsed = new URL(candidate, LOCAL_BASE);
    if (parsed.origin !== LOCAL_BASE || !parsed.pathname.startsWith("/")) {
      return fallback;
    }
    return `${parsed.pathname}${parsed.search}`;
  } catch {
    return fallback;
  }
}
