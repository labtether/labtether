/**
 * A random id for things that live only in this browser tab (Files and Remote View tabs, a failed transfer row).
 * crypto.randomUUID exists only in secure contexts, and the console is often opened over plain HTTP on a LAN address
 * (http://192.0.2.10:3000), where calling it throws. getRandomValues works in every context, so build a version 4
 * UUID from it there.
 */
export function clientId(): string {
  const c: Crypto | undefined = typeof crypto === "undefined" ? undefined : crypto;
  if (typeof c?.randomUUID === "function") return c.randomUUID();
  const bytes = new Uint8Array(16);
  if (typeof c?.getRandomValues === "function") c.getRandomValues(bytes);
  else for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
