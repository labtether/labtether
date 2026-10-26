// @ts-check
// HTTP helpers shared by every route. Responses never echo request content,
// never include stack traces and always carry a constant JSON shape.

/** Largest request body the relay will read, checked before JSON parsing. */
export const MAX_BODY_BYTES = 8 * 1024;

const BASE_HEADERS = Object.freeze({
  "cache-control": "no-store",
  "x-content-type-options": "nosniff",
});

/**
 * @param {number} status
 * @param {Record<string, unknown>} body
 * @param {Record<string, string>} [extraHeaders]
 * @returns {Response}
 */
export function json(status, body, extraHeaders = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      ...BASE_HEADERS,
      "content-type": "application/json; charset=utf-8",
      ...extraHeaders,
    },
  });
}

/** @returns {Response} */
export function noContent() {
  return new Response(null, { status: 204, headers: { ...BASE_HEADERS } });
}

/**
 * @param {number} status
 * @param {string} code
 * @param {Record<string, string>} [extraHeaders]
 */
export function errorResponse(status, code, extraHeaders) {
  return json(status, { error: code }, extraHeaders);
}

/**
 * @param {string[]} allowed
 * @returns {Response}
 */
export function methodNotAllowed(allowed) {
  return errorResponse(405, "method_not_allowed", { allow: allowed.join(", ") });
}

/**
 * @typedef {{ ok: true, value: unknown } | { ok: false, response: Response }} BodyResult
 */

/**
 * Reads a JSON request body with a hard byte cap. The content type, declared
 * length and streamed length are all checked before any parsing happens.
 *
 * @param {Request} request
 * @param {number} [maxBytes]
 * @returns {Promise<BodyResult>}
 */
export async function readJsonBody(request, maxBytes = MAX_BODY_BYTES) {
  if (!isJsonContentType(request.headers.get("content-type"))) {
    return { ok: false, response: errorResponse(415, "unsupported_media_type") };
  }
  const declared = request.headers.get("content-length");
  if (declared !== null) {
    const length = Number(declared);
    if (!/^\d+$/.test(declared.trim()) || !Number.isSafeInteger(length)) {
      return { ok: false, response: errorResponse(400, "invalid_request") };
    }
    if (length > maxBytes) {
      return { ok: false, response: errorResponse(413, "body_too_large") };
    }
  }
  const bytes = await readBounded(request.body, maxBytes);
  if (bytes === null) {
    return { ok: false, response: errorResponse(413, "body_too_large") };
  }
  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return { ok: true, value: JSON.parse(text) };
  } catch {
    return { ok: false, response: errorResponse(400, "invalid_json") };
  }
}

/**
 * Accepts `application/json` with no charset or a UTF-8 charset only.
 *
 * @param {string | null} header
 */
export function isJsonContentType(header) {
  if (!header) return false;
  const [mediaType, ...params] = header.split(";").map((part) => part.trim().toLowerCase());
  if (mediaType !== "application/json") return false;
  return params.every((param) => {
    const [name, value] = param.split("=").map((part) => part.trim());
    return name !== "charset" || value === "utf-8" || value === '"utf-8"';
  });
}

/**
 * Streams at most `maxBytes` bytes. Returns null as soon as the cap is passed.
 *
 * @param {ReadableStream<Uint8Array> | null} stream
 * @param {number} maxBytes
 * @returns {Promise<Uint8Array | null>}
 */
async function readBounded(stream, maxBytes) {
  if (!stream) return new Uint8Array(0);
  const reader = stream.getReader();
  /** @type {Uint8Array[]} */
  const chunks = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > maxBytes) {
      await reader.cancel().catch(() => {});
      return null;
    }
    chunks.push(value);
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}
