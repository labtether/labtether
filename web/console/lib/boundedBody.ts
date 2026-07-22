export class RequestBodyTooLargeError extends Error {
  constructor() {
    super("request body too large");
    this.name = "RequestBodyTooLargeError";
  }
}

/**
 * Read a request body without ever retaining more than maxBytes. The
 * Content-Length check is only a fast rejection; the streaming byte counter is
 * authoritative for chunked requests and callers that omit or forge the
 * header.
 */
export async function readBoundedRequestBody(request: Request, maxBytes: number): Promise<ArrayBuffer> {
  const declaredLength = request.headers.get("content-length")?.trim() ?? "";
  if (/^\d+$/.test(declaredLength) && Number(declaredLength) > maxBytes) {
    throw new RequestBodyTooLargeError();
  }

  if (!request.body) {
    return new ArrayBuffer(0);
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      if (value.byteLength > maxBytes - total) {
        await reader.cancel();
        throw new RequestBodyTooLargeError();
      }
      total += value.byteLength;
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body.buffer;
}
