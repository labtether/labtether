"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { PageError } from "../../../components/ui/PageError";

export default function UsersError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("common");
  const tSection = useTranslations("nav");

  useEffect(() => {
    console.error("Users section error:", error);
  }, [error]);

  return (
    <PageError
      title={t("errorBoundary.sectionTitle", { section: tSection("users") })}
      message={t("errorBoundary.sectionDescription")}
      retryLabel={t("errorBoundary.tryAgain")}
      onRetry={reset}
    />
  );
}
