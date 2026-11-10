/**
 * Certificate fingerprints (SHA-256 of the certificate's DER bytes), in the one form every LabTether surface shows:
 * lowercase hex in groups of four with spaces ("3f9a 1c0e ..."). The console, the Mac, Windows and iPhone apps must
 * print the same characters, because the person compares them by eye before trusting a Hub.
 */

/** The fingerprint as plain lowercase hex (64 characters), or "" when the value is not a SHA-256 fingerprint. */
export function plainFingerprint(raw: string | null | undefined): string {
  const hex = (raw ?? "").replace(/[\s:]/g, "").toLowerCase();
  return /^[0-9a-f]{64}$/.test(hex) ? hex : "";
}

/** The fingerprint in 16 groups of four, or "" when the value is not a SHA-256 fingerprint. */
export function groupedFingerprint(raw: string | null | undefined): string {
  return plainFingerprint(raw).replace(/(.{4})(?=.)/g, "$1 ");
}
