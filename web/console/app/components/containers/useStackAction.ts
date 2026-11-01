"use client";

import { useTranslations } from "next-intl";
import { useCallback } from "react";
import { useToast } from "../../contexts/ToastContext";
import { executeStackAction } from "../../../lib/docker";

export type StackAction = "stack.up" | "stack.restart" | "stack.down";
const WORD: Record<StackAction, "up" | "restart" | "down"> = { "stack.up": "up", "stack.restart": "restart", "stack.down": "down" };

/** Runs a compose action on one stack (by its asset id) with a toast, then refreshes after the host has caught up. */
export function useStackAction(name: string, stackAssetId: string, onChanged?: () => void) {
  const t = useTranslations("devices-docker.stacks.toast");
  const { addToast } = useToast();
  return useCallback(async (action: StackAction) => {
    try {
      const result = await executeStackAction(stackAssetId, action);
      if (result.status === "failed") throw new Error(result.message);
      addToast("success", t(`${WORD[action]}Done`, { name }));
      setTimeout(() => onChanged?.(), 1400);
    } catch (err) {
      const why = err instanceof Error && err.message ? `: ${err.message}` : "";
      addToast("error", `${t(`${WORD[action]}Failed`, { name })}${why}`);
    }
  }, [addToast, name, onChanged, stackAssetId, t]);
}
