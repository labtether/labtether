import type { InputStream, OutputStream } from "guacamole-common-js";

// Guacamole 1.5's StringWriter encodes UTF-16 code units separately. Encode
// complete Unicode text once, then split its UTF-8 bytes at the stream boundary.
export function writeGuacamoleClipboard(stream: OutputStream, text: string): void {
  const bytes = new TextEncoder().encode(text);
  for (let offset = 0; offset < bytes.length; offset += 6048) {
    let binary = "";
    for (const byte of bytes.subarray(offset, offset + 6048)) binary += String.fromCharCode(byte);
    stream.sendBlob(btoa(binary));
  }
  // The RDP clipboard handlers do not send ACKs, including at stream creation.
  stream.sendEnd();
}

export function readGuacamoleClipboard(stream: InputStream, onText: (text: string) => void): () => void {
  const decoder = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true });
  let text = "";
  let active = true;
  const cleanup = () => {
    active = false;
    text = "";
    stream.onblob = null;
    stream.onend = null;
  };
  stream.onblob = (base64) => {
    if (!active) return;
    try {
      const binary = atob(base64);
      const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
      text += decoder.decode(bytes, { stream: true });
      stream.sendAck("OK", 0);
    } catch {
      cleanup();
      stream.sendAck("Invalid UTF-8 clipboard data", 0x0300);
    }
  };
  stream.onend = () => {
    if (!active) return;
    try {
      const complete = text + decoder.decode();
      cleanup();
      onText(complete);
    } catch {
      cleanup();
    }
  };
  return cleanup;
}
