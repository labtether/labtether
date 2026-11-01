"use client";

import { useTranslations } from "next-intl";

/** What is left of the full-screen toolbar while it is hidden: a small tab on the screen's edge that brings it back. */
export function ToolbarRevealHandle({ position, onReveal }: { position: "top" | "bottom"; onReveal: () => void }) {
  const t = useTranslations("desktop.tools");
  return (
    <button
      type="button"
      className={`rv-handle ${position}`}
      onClick={onReveal}
      onMouseEnter={onReveal}
      onFocus={onReveal}
      aria-label={t("showTools")}
      title={t("showTools")}
    >
      <i aria-hidden="true" />
    </button>
  );
}
