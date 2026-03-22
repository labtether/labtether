// NOTE: Root error boundary is outside [locale] — no IntlProvider available.
// Hardcoded English is intentional here; the console-level error.tsx handles the i18n case.
"use client";

export default function RootError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "100vh",
        gap: "1rem",
        padding: "2rem",
        background: "var(--bg, #0a0a0c)",
      }}
    >
      <h2
        style={{
          fontSize: "1.125rem",
          fontWeight: 600,
          color: "var(--bad, #f87171)",
          margin: 0,
        }}
      >
        Something went wrong
      </h2>
      <p
        style={{
          fontSize: "0.875rem",
          color: "var(--muted, rgba(255,255,255,0.4))",
          margin: 0,
          textAlign: "center",
          maxWidth: "40ch",
        }}
      >
        {error.message}
      </p>
      <button
        onClick={reset}
        style={{
          padding: "0.5rem 1rem",
          borderRadius: "6px",
          border: "1px solid var(--line, rgba(255,255,255,0.08))",
          background: "var(--surface, rgba(255,255,255,0.04))",
          color: "var(--text, rgba(255,255,255,0.85))",
          cursor: "pointer",
          fontSize: "0.875rem",
        }}
      >
        Try again
      </button>
    </div>
  );
}
