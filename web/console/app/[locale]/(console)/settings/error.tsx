"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { PageError } from "../../../components/ui/PageError";

export default function SettingsError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("common");

  useEffect(() => {
    console.error("Settings section error:", error);
  }, [error]);

  return (
    <PageError
      title={t("errorBoundary.sectionTitle", { section: "Settings" })}
      message={t("errorBoundary.sectionDescription")}
      retryLabel={t("errorBoundary.tryAgain")}
      onRetry={reset}
    />
  );
}
