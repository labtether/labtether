"use client";

import { Link2 } from "lucide-react";
import { Button } from "../../../components/ui/Button";

type LinkSuggestionsBannerProps = {
  count: number;
  onReview: () => void;
};

export function LinkSuggestionsBanner({ count, onReview }: LinkSuggestionsBannerProps) {
  if (count === 0) return null;

  return (
    <div
      className="mb-4 flex items-center justify-between gap-3 rounded-lg border border-[var(--accent)]/30 bg-[var(--accent)]/5 px-4 py-2.5"
      style={{
        backdropFilter: "blur(var(--blur-sm)) saturate(1.5)",
        WebkitBackdropFilter: "blur(var(--blur-sm)) saturate(1.5)",
        boxShadow: "0 0 12px var(--accent-glow)",
      }}
    >
      <div className="flex items-center gap-2.5">
        <Link2
          size={16}
          className="shrink-0 text-[var(--accent)]"
        />
        <span className="text-sm font-medium text-[var(--text)]">
          {count} suggested relationship{count !== 1 ? "s" : ""}
        </span>
      </div>
      <Button variant="primary" size="sm" onClick={onReview}>
        Review
      </Button>
    </div>
  );
}
