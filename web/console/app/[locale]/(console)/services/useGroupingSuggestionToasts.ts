"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef } from "react";
import { useToast } from "../../../contexts/ToastContext";
import type { URLGroupingSuggestion } from "../../../hooks/useWebServices";

/**
 * Asks once per suggestion whether a found URL is another address of a service, with Accept and Deny in the toast.
 * A failed answer is left for the next poll, which still lists the suggestion.
 */
export function useGroupingSuggestionToasts(suggestions: URLGroupingSuggestion[], refresh: () => Promise<void>) {
  const t = useTranslations("services.suggestions");
  const { addToast } = useToast();
  const shown = useRef<Set<string>>(new Set());

  useEffect(() => {
    for (const suggestion of suggestions) {
      if (shown.current.has(suggestion.id)) continue;
      shown.current.add(suggestion.id);
      const answer = (verb: "accept" | "deny") => async () => {
        try {
          await fetch(`/api/services/web/grouping-suggestions/${encodeURIComponent(suggestion.id)}/${verb}`, { method: "POST" });
          await refresh();
        } catch {
          // The next poll lists the suggestion again.
        }
      };
      addToast("info", t("ask", { url: suggestion.suggested_url, name: suggestion.base_service_name }), 15000,
        { label: t("accept"), onClick: answer("accept") },
        { label: t("deny"), onClick: answer("deny") });
    }
  }, [addToast, refresh, suggestions, t]);
}
