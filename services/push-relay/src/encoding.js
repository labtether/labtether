// @ts-check
// Small byte encoders shared by the APNs signer and the registration store.

/**
 * @param {Uint8Array} bytes
 * @returns {string} unpadded base64url
 */
export function base64url(bytes) {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * @param {ArrayBuffer | Uint8Array} buffer
 * @returns {string} lowercase hex
 */
export function hex(buffer) {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}
