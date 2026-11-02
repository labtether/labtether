"use client";

import { useTranslations } from "next-intl";
import { PageError } from "../components/ui/PageError";

/** While the first session check runs: the empty canvas, no words. */
export function SessionChecking() {
  return <div className="min-h-screen bg-[var(--bg)]" aria-busy="true" />;
}

/** The hub did not answer the session check. AuthProvider keeps checking on a timer; Try again checks now. */
export function SessionUnreachable({ onRetry }: { onRetry: () => void }) {
  const t = useTranslations("common");
  return (
    <div className="grid min-h-screen place-items-center bg-[var(--bg)] px-6">
      <PageError
        title={t("session.unreachable")}
        message={t("session.retrying")}
        retryLabel={t("errorBoundary.tryAgain")}
        onRetry={onRetry}
      />
    </div>
  );
}
