"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { PageError } from "../../../components/ui/PageError";

export default function ApiDocsError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("common");
  const tSection = useTranslations("nav");

  useEffect(() => {
    console.error("API docs section error:", error);
  }, [error]);

  return (
    <PageError
      title={t("errorBoundary.sectionTitle", { section: tSection("apiDocs") })}
      message={t("errorBoundary.sectionDescription")}
      retryLabel={t("errorBoundary.tryAgain")}
      onRetry={reset}
    />
  );
}
