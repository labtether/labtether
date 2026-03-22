"use client";

import { useCallback, useEffect, useState } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import {
  buildCategoryLayoutOrder,
  moveArrayItem,
  SERVICE_LAYOUT_STORAGE_KEY,
  serviceLayoutKey,
  type ServiceLayoutState,
} from "./servicesPageHelpers";

interface UseServiceLayoutOrchestrationArgs {
  services: WebService[];
}

export function useServiceLayoutOrchestration({
  services,
}: UseServiceLayoutOrchestrationArgs) {
  const [layoutMode, setLayoutMode] = useState(false);
  const [layoutOrderByCategory, setLayoutOrderByCategory] = useState<ServiceLayoutState>({});
  const [draggingServiceKey, setDraggingServiceKey] = useState<string | null>(null);
  const [draggingCategory, setDraggingCategory] = useState<string | null>(null);
  const [dragOverServiceKey, setDragOverServiceKey] = useState<string | null>(null);

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(SERVICE_LAYOUT_STORAGE_KEY);
      if (!raw) {
        return;
      }
      const parsed = JSON.parse(raw) as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        return;
      }

      const next: ServiceLayoutState = {};
      for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
        if (!Array.isArray(value)) {
          continue;
        }
        const normalized = value
          .map((entry) => (typeof entry === "string" ? entry.trim() : ""))
          .filter((entry) => entry.length > 0);
        if (normalized.length === 0) {
          continue;
        }
        next[key] = normalized;
      }
      setLayoutOrderByCategory(next);
    } catch {
      // ignore local layout parse errors
    }
  }, []);

  const persistLayoutOrder = useCallback((next: ServiceLayoutState) => {
    try {
      if (Object.keys(next).length === 0) {
        window.localStorage.removeItem(SERVICE_LAYOUT_STORAGE_KEY);
        return;
      }
      window.localStorage.setItem(SERVICE_LAYOUT_STORAGE_KEY, JSON.stringify(next));
    } catch {
      // ignore local layout persistence errors
    }
  }, []);

  const clearDragState = useCallback(() => {
    setDraggingServiceKey(null);
    setDraggingCategory(null);
    setDragOverServiceKey(null);
  }, []);

  useEffect(() => {
    if (!layoutMode) {
      clearDragState();
    }
  }, [layoutMode, clearDragState]);

  const handleCardDragStart = useCallback(
    (service: WebService) => {
      if (!layoutMode) {
        return;
      }
      setDraggingServiceKey(serviceLayoutKey(service));
      setDraggingCategory(service.category);
      setDragOverServiceKey(null);
    },
    [layoutMode]
  );

  const handleCardDragOver = useCallback(
    (service: WebService) => {
      if (!layoutMode || !draggingServiceKey || !draggingCategory) {
        return;
      }
      if (service.category !== draggingCategory) {
        setDragOverServiceKey((current) => (current === null ? current : null));
        return;
      }
      const key = serviceLayoutKey(service);
      if (key === draggingServiceKey) {
        setDragOverServiceKey((current) => (current === null ? current : null));
        return;
      }
      setDragOverServiceKey((current) => (current === key ? current : key));
    },
    [layoutMode, draggingCategory, draggingServiceKey]
  );

  const handleCardDrop = useCallback(
    (targetService: WebService) => {
      if (!layoutMode || !draggingServiceKey || !draggingCategory) {
        return;
      }

      const targetCategory = targetService.category;
      const targetKey = serviceLayoutKey(targetService);
      if (targetCategory !== draggingCategory || targetKey === draggingServiceKey) {
        clearDragState();
        return;
      }

      setLayoutOrderByCategory((current) => {
        const currentOrder = current[targetCategory] ?? [];
        const baseOrder = buildCategoryLayoutOrder(targetCategory, services, currentOrder);
        const fromIndex = baseOrder.indexOf(draggingServiceKey);
        const toIndex = baseOrder.indexOf(targetKey);
        if (fromIndex < 0 || toIndex < 0) {
          return current;
        }

        const moved = moveArrayItem(baseOrder, fromIndex, toIndex);
        if (moved === baseOrder) {
          return current;
        }
        const next: ServiceLayoutState = {
          ...current,
          [targetCategory]: moved,
        };
        persistLayoutOrder(next);
        return next;
      });

      clearDragState();
    },
    [layoutMode, draggingServiceKey, draggingCategory, services, clearDragState, persistLayoutOrder]
  );

  const resetLayoutOrder = useCallback(() => {
    setLayoutOrderByCategory({});
    persistLayoutOrder({});
    clearDragState();
  }, [clearDragState, persistLayoutOrder]);

  return {
    layoutMode,
    setLayoutMode,
    layoutOrderByCategory,
    draggingServiceKey,
    dragOverServiceKey,
    clearDragState,
    handleCardDragStart,
    handleCardDragOver,
    handleCardDrop,
    resetLayoutOrder,
  };
}
