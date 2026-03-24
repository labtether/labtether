"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";

export default function GroupsError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("common");

  useEffect(() => {
    console.error("Groups section error:", error);
  }, [error]);

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "50vh",
        gap: "1rem",
        padding: "2rem",
      }}
    >
      <h2
        style={{
          fontSize: "1.125rem",
          fontWeight: 600,
          color: "var(--bad)",
          margin: 0,
        }}
      >
        {t("errorBoundary.sectionTitle", { section: "Groups" })}
      </h2>
      <p
        style={{
          fontSize: "0.875rem",
          color: "var(--muted)",
          margin: 0,
          textAlign: "center",
          maxWidth: "40ch",
        }}
      >
        {t("errorBoundary.sectionDescription")}
      </p>
      <button
        onClick={reset}
        style={{
          padding: "0.5rem 1rem",
          borderRadius: "var(--radius-sm)",
          border: "1px solid var(--line)",
          background: "var(--surface)",
          color: "var(--text)",
          cursor: "pointer",
          fontSize: "0.875rem",
          transition: "background var(--dur-fast) var(--ease-out)",
        }}
      >
        {t("errorBoundary.tryAgain")}
      </button>
    </div>
  );
}
