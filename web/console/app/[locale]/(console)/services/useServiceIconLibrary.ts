"use client";

import { useCallback, useEffect, useState } from "react";
import type {
  ServiceCustomIcon,
  ServiceCustomIconInput,
  ServiceCustomIconRenameInput,
} from "../../../hooks/useWebServices";

interface UseServiceIconLibraryArgs {
  listCustomServiceIcons: () => Promise<ServiceCustomIcon[]>;
  createCustomServiceIcon: (input: ServiceCustomIconInput) => Promise<ServiceCustomIcon>;
  deleteCustomServiceIcon: (id: string) => Promise<void>;
  renameCustomServiceIcon: (input: ServiceCustomIconRenameInput) => Promise<ServiceCustomIcon>;
}

export function useServiceIconLibrary({
  listCustomServiceIcons,
  createCustomServiceIcon,
  deleteCustomServiceIcon,
  renameCustomServiceIcon,
}: UseServiceIconLibraryArgs) {
  const [iconIndex, setIconIndex] = useState<string[]>([]);
  const [customIcons, setCustomIcons] = useState<ServiceCustomIcon[]>([]);

  useEffect(() => {
    fetch("/service-icons/index.json")
      .then((r) => r.json())
      .then((data) => setIconIndex(data as string[]))
      .catch(() => {});
  }, []);

  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const items = await listCustomServiceIcons();
        if (!active) {
          return;
        }
        setCustomIcons(items);
      } catch {
        if (!active) {
          return;
        }
        setCustomIcons([]);
      }
    })();
    return () => {
      active = false;
    };
  }, [listCustomServiceIcons]);

  const createCustomIcon = useCallback(
    async (input: ServiceCustomIconInput): Promise<ServiceCustomIcon> => {
      const created = await createCustomServiceIcon(input);
      setCustomIcons((current) => {
        const next = current.filter((icon) => icon.id !== created.id);
        next.push(created);
        return next;
      });
      return created;
    },
    [createCustomServiceIcon]
  );

  const deleteCustomIcon = useCallback(
    async (id: string) => {
      await deleteCustomServiceIcon(id);
      setCustomIcons((current) => current.filter((icon) => icon.id !== id));
    },
    [deleteCustomServiceIcon]
  );

  const renameCustomIcon = useCallback(
    async (id: string, name: string): Promise<ServiceCustomIcon> => {
      const renamed = await renameCustomServiceIcon({ id, name });
      setCustomIcons((current) =>
        current.map((icon) => (icon.id === renamed.id ? renamed : icon))
      );
      return renamed;
    },
    [renameCustomServiceIcon]
  );

  return {
    iconIndex,
    customIcons,
    createCustomIcon,
    deleteCustomIcon,
    renameCustomIcon,
  };
}
