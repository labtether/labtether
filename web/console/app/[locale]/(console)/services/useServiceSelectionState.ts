"use client";

import { useCallback, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import { serviceLayoutKey } from "./servicesPageHelpers";

interface UseServiceSelectionStateArgs {
  layoutMode: boolean;
  setLayoutMode: Dispatch<SetStateAction<boolean>>;
  clearDragState: () => void;
}

export function useServiceSelectionState({
  layoutMode,
  setLayoutMode,
  clearDragState,
}: UseServiceSelectionStateArgs) {
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedServiceKeys, setSelectedServiceKeys] = useState<Set<string>>(new Set());

  const reconcileSelectionWithFiltered = useCallback((filtered: WebService[]) => {
    setSelectedServiceKeys((current) => {
      if (current.size === 0) {
        return current;
      }
      const allowed = new Set(filtered.map((service) => serviceLayoutKey(service)));
      const next = new Set<string>();
      for (const key of current) {
        if (allowed.has(key)) {
          next.add(key);
        }
      }
      if (next.size === current.size) {
        return current;
      }
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedServiceKeys(new Set());
  }, []);

  const disableSelectionMode = useCallback(() => {
    setSelectionMode(false);
    clearSelection();
  }, [clearSelection]);

  const handleToggleSelectionMode = useCallback(() => {
    setSelectionMode((current) => {
      const next = !current;
      if (!next) {
        clearSelection();
      } else if (layoutMode) {
        setLayoutMode(false);
        clearDragState();
      }
      return next;
    });
  }, [clearDragState, clearSelection, layoutMode, setLayoutMode]);

  const handleToggleServiceSelection = useCallback((service: WebService) => {
    const key = serviceLayoutKey(service);
    setSelectedServiceKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const handleSelectAllFiltered = useCallback((filtered: WebService[]) => {
    setSelectedServiceKeys(new Set(filtered.map((service) => serviceLayoutKey(service))));
  }, []);

  return {
    selectionMode,
    setSelectionMode,
    selectedServiceKeys,
    reconcileSelectionWithFiltered,
    clearSelection,
    disableSelectionMode,
    handleToggleSelectionMode,
    handleToggleServiceSelection,
    handleSelectAllFiltered,
  };
}
