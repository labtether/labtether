"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

// --- Public types ---

export interface PaletteItem {
  id: string;
  label: string;
  description?: string;
  icon?: LucideIcon;
  href?: string;
  action: () => void;
  keywords?: string[];
}

export interface PaletteProvider {
  id: string;
  group: string;
  priority: number;
  search(query: string): PaletteItem[];
  shortcut?: string;
}

// --- Context value ---

interface PaletteContextValue {
  /** Read the current registry snapshot (ref-based, no re-render on mutation). */
  getProviders: () => PaletteProvider[];
  /** Register a provider. Returns an unregister function. */
  register: (provider: PaletteProvider) => () => void;
  open: boolean;
  setOpen: (open: boolean | ((prev: boolean) => boolean)) => void;
}

const PaletteContext = createContext<PaletteContextValue | null>(null);

// --- Provider component ---

export function PaletteContextProvider({ children }: { children: ReactNode }) {
  const registryRef = useRef<Map<string, PaletteProvider>>(new Map());
  const [open, setOpen] = useState(false);

  const getProviders = useCallback((): PaletteProvider[] => {
    return Array.from(registryRef.current.values()).sort((a, b) => a.priority - b.priority);
  }, []);

  const register = useCallback((provider: PaletteProvider): (() => void) => {
    registryRef.current.set(provider.id, provider);
    return () => {
      registryRef.current.delete(provider.id);
    };
  }, []);

  const value: PaletteContextValue = { getProviders, register, open, setOpen };

  return (
    <PaletteContext.Provider value={value}>
      {children}
    </PaletteContext.Provider>
  );
}

// --- Hooks ---

export function usePalette(): PaletteContextValue {
  const ctx = useContext(PaletteContext);
  if (!ctx) {
    throw new Error("usePalette must be used within a PaletteContextProvider");
  }
  return ctx;
}

/**
 * Register a PaletteProvider for the lifetime of the calling component.
 * Automatically unregisters on unmount.
 */
export function usePaletteRegister(provider: PaletteProvider): void {
  const { register } = usePalette();
  const providerRef = useRef(provider);
  providerRef.current = provider;

  useEffect(() => {
    const stable: PaletteProvider = {
      id: provider.id,
      group: provider.group,
      priority: providerRef.current.priority,
      shortcut: providerRef.current.shortcut,
      search: (q: string) => providerRef.current.search(q),
    };
    return register(stable);
  }, [register, provider.id, provider.group, providerRef]);
}
