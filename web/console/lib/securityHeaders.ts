export function consoleContentSecurityPolicy(isDevelopment: boolean): string {
  return [
    "default-src 'self'",
    // Next emits an inline bootstrap script. Replacing unsafe-inline requires
    // a per-request nonce propagated through every App Router render.
    `script-src 'self' 'unsafe-inline'${isDevelopment ? " 'unsafe-eval'" : ""}`,
    "script-src-attr 'none'",
    // The console uses React style props extensively; external styles are
    // limited to the terminal font stylesheet.
    "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    "font-src 'self' data: https://fonts.gstatic.com",
    "img-src 'self' data: blob:",
    `connect-src 'self'${isDevelopment ? " ws://localhost:* ws://127.0.0.1:*" : ""}`,
    "media-src 'self' blob:",
    "worker-src 'self' blob:",
    "manifest-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-src 'none'",
    "frame-ancestors 'none'",
  ].join("; ");
}

export function consoleSecurityHeaders(isDevelopment: boolean): Array<{ key: string; value: string }> {
  return [
    { key: "Content-Security-Policy", value: consoleContentSecurityPolicy(isDevelopment) },
    { key: "X-Content-Type-Options", value: "nosniff" },
    { key: "X-Frame-Options", value: "DENY" },
    { key: "X-Permitted-Cross-Domain-Policies", value: "none" },
    { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
    { key: "X-DNS-Prefetch-Control", value: "on" },
    { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
    // Browsers enforce HSTS only on HTTPS responses, so local HTTP development
    // remains usable.
    { key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" },
  ];
}
