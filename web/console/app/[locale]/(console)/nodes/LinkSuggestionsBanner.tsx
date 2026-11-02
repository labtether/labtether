"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";

type LinkSuggestionsBannerProps = {
  count: number;
  onReview: () => void;
};

/** One quiet line above the device list while discovery has suggested relationships; Review opens them. */
export function LinkSuggestionsBanner({ count, onReview }: LinkSuggestionsBannerProps) {
  const t = useTranslations("devices.proposals");
  if (count === 0) return null;

  return (
    <div className="ld-wait line">
      <p className="ld-wait-h">{t("newSuggestions", { count })}</p>
      <Button size="sm" onClick={onReview}>
        {t("review")}
      </Button>
    </div>
  );
}
