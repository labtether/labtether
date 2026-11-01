"use client";

import { useCallback, useEffect, useState } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import { serviceLayoutKey } from "./servicesLayoutHelpers";

/** Picked rows, by layout key. Picks of services the list no longer shows are dropped. */
export function useServiceSelectionState(visible: WebService[]) {
  const [selected, setSelected] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    setSelected((current) => {
      if (current.size === 0) return current;
      const allowed = new Set(visible.map(serviceLayoutKey));
      const next = new Set([...current].filter((key) => allowed.has(key)));
      return next.size === current.size ? current : next;
    });
  }, [visible]);

  const toggle = useCallback((service: WebService) => {
    const key = serviceLayoutKey(service);
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  }, []);

  const setAll = useCallback((on: boolean) => {
    setSelected(on ? new Set(visible.map(serviceLayoutKey)) : new Set());
  }, [visible]);

  const clear = useCallback(() => setSelected(new Set()), []);

  return { selected, toggle, setAll, clear };
}
