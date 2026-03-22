/**
 * Extends RequestInit with the `duplex` property supported by Node.js fetch
 * for streaming request bodies. This avoids repeated @ts-expect-error
 * suppressions across API proxy routes.
 *
 * @see https://fetch.spec.whatwg.org/#dom-requestinit-duplex
 */
interface RequestInit {
  duplex?: "half";
}
