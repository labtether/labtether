"use client";

import { useTranslations } from "next-intl";
import { PageError } from "../../components/ui/PageError";
import { sanitizeErrorMessage } from "../../lib/sanitizeErrorMessage";

export default function ConsoleError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("common");

  return (
    <PageError
      title={t("errorBoundary.title")}
      message={sanitizeErrorMessage(error.message, "")}
      retryLabel={t("errorBoundary.tryAgain")}
      onRetry={reset}
    />
  );
}
