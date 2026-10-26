// Linear-time edge trimming. Regexes such as /\/+$/ or /\.+$/ restart the
// run from every position on input like "////…x", which takes quadratic time
// (CodeQL js/polynomial-redos) when the input comes from users or requests.

export function trimTrailingChars(value: string, chars: string): string {
  let end = value.length;
  while (end > 0 && chars.includes(value.charAt(end - 1))) {
    end -= 1;
  }
  return end === value.length ? value : value.slice(0, end);
}

export function trimLeadingChars(value: string, chars: string): string {
  let start = 0;
  while (start < value.length && chars.includes(value.charAt(start))) {
    start += 1;
  }
  return start === 0 ? value : value.slice(start);
}

export function trimChars(value: string, chars: string): string {
  return trimLeadingChars(trimTrailingChars(value, chars), chars);
}

export function trimTrailingSlashes(value: string): string {
  return trimTrailingChars(value, "/");
}
